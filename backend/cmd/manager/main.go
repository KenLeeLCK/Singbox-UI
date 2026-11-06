package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/api"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/core"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/telemetry"
	"github.com/sing-box-manager/sing-box-manager/backend/internal/web"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	mode := "serve"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "serve" || args[0] == "helper") {
		mode = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet(mode, flag.ExitOnError)
	state := f.String("state-dir", ".manager", "restricted state directory")
	config := f.String("config", "", "explicit sing-box config file")
	binary := f.String("sing-box", "", "sing-box executable")
	unit := f.String("unit", "sing-box.service", "fixed systemd unit")
	wd := f.String("working-dir", "", "sing-box working directory")
	local := f.Bool("local", false, "local inspect/validate only (no systemd changes)")
	socket := f.String("socket", "/run/sing-box-manager/helper.sock", "privileged helper socket")
	remote := f.String("helper-socket", "", "connect to privileged helper")
	group := f.String("socket-group", "sing-box-manager", "socket group")
	port := f.Int("port", 5030, "preferred loopback port")
	max := f.Int("port-max", 5099, "last permitted port")
	f.Parse(args)
	if *port < 1024 || *max > 65535 || *max < *port {
		log.Fatal("invalid port search range")
	}
	if !strings.HasSuffix(*unit, ".service") || strings.ContainsAny(*unit, "/\n \t") {
		log.Fatal("invalid service unit")
	}
	if er := os.MkdirAll(*state, 0700); er != nil {
		log.Fatal(er)
	}
	lock, er := os.OpenFile(filepath.Join(*state, "instance.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if er != nil {
		log.Fatal(er)
	}
	if er = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); er != nil {
		log.Fatal("another manager process owns this state directory")
	}
	defer lock.Close()
	var eng *core.Engine
	if mode == "helper" || *remote == "" {
		eng, er = core.New(core.Options{StateDir: *state, Config: *config, Binary: *binary, Unit: *unit, WorkingDir: *wd, Local: *local})
		if er != nil {
			log.Fatal(er)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var handler http.Handler
	var listener net.Listener
	if mode == "helper" {
		st, err := os.Lstat(*state)
		if err != nil {
			log.Fatal(err)
		}
		sys, ok := st.Sys().(*syscall.Stat_t)
		if !ok || sys.Uid != 0 || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
			log.Fatal("helper state must be root-owned directory with mode 0700")
		}
		if os.Geteuid() != 0 {
			log.Fatal("helper requires root")
		}
		if er = eng.Recover(); er != nil {
			log.Fatalf("pending deployment recovery: %v", er)
		}
		if er = os.MkdirAll(filepath.Dir(*socket), 0750); er != nil {
			log.Fatal(er)
		}
		g, er := user.LookupGroup(*group)
		if er != nil {
			log.Fatal(er)
		}
		gid, _ := strconv.Atoi(g.Gid)
		os.Chown(filepath.Dir(*socket), 0, gid)
		if st, er := os.Lstat(*socket); er == nil {
			if st.Mode()&os.ModeSocket == 0 {
				log.Fatal("socket path is not socket")
			}
			os.Remove(*socket)
		}
		listener, er = net.Listen("unix", *socket)
		if er != nil {
			log.Fatal(er)
		}
		if er = os.Chown(*socket, 0, gid); er != nil {
			log.Fatal(er)
		}
		if er = os.Chmod(*socket, 0660); er != nil {
			log.Fatal(er)
		}
		handler = (&api.Server{Engine: eng}).Internal()
	} else {
		if os.Geteuid() == 0 {
			log.Fatal("web server must run as dedicated non-root user")
		}
		s, er := api.New(eng, *state, *remote)
		if er != nil {
			log.Fatal(er)
		}
		monitor := telemetry.New(s.Config)
		s.Telemetry = http.HandlerFunc(monitor.Handler)
		go monitor.Run(ctx)
		var saved struct {
			Port int `json:"port"`
		}
		if b, er := os.ReadFile(filepath.Join(*state, "selected-port.json")); er == nil {
			json.Unmarshal(b, &saved)
		}
		ports := []int{}
		if saved.Port >= *port && saved.Port <= *max {
			ports = append(ports, saved.Port)
		}
		for p := *port; p <= *max; p++ {
			if p != saved.Port {
				ports = append(ports, p)
			}
		}
		for _, p := range ports {
			listener, er = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
			if er == nil {
				saved.Port = p
				break
			}
		}
		if listener == nil {
			log.Fatalf("no free loopback port in %d–%d; free a port or adjust --port-max", *port, *max)
		}
		b, _ := json.Marshal(saved)
		if er = core.Atomic(filepath.Join(*state, "selected-port.json"), b, 0600); er != nil {
			log.Fatal(er)
		}
		mux := http.NewServeMux()
		mux.Handle("/api/", s.Handler())
		mux.Handle("/healthz", s.Handler())
		mux.Handle("/", web.Handler())
		handler = mux
		log.Printf("Manager listening on http://127.0.0.1:%d; login token in %s", saved.Port, filepath.Join(*state, "token"))
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16384}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(c)
	}()
	if er = srv.Serve(listener); er != nil && er != http.ErrServerClosed {
		log.Fatal(er)
	}
}
