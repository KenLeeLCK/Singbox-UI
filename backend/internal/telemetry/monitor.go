// Package telemetry bridges the read-only sing-box 1.14 native gRPC API to SSE.
package telemetry

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sing-box-manager/sing-box-manager/backend/internal/telemetry/wire"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Connection struct {
	ID            string `json:"id"`
	CreatedAt     int64  `json:"createdAt"`
	Inbound       string `json:"inbound"`
	Outbound      string `json:"outbound"`
	Network       string `json:"network"`
	Source        string `json:"source"`
	Destination   string `json:"destination"`
	Domain        string `json:"domain"`
	Protocol      string `json:"protocol"`
	UploadTotal   int64  `json:"uploadTotal"`
	DownloadTotal int64  `json:"downloadTotal"`
}

type Snapshot struct {
	Available            bool         `json:"available"`
	Error                string       `json:"error,omitempty"`
	Timestamp            int64        `json:"timestamp"`
	Version              string       `json:"version,omitempty"`
	UploadRate           *float64     `json:"uploadRate"`
	DownloadRate         *float64     `json:"downloadRate"`
	UploadTotal          *int64       `json:"uploadTotal"`
	DownloadTotal        *int64       `json:"downloadTotal"`
	ActiveConnections    *int32       `json:"activeConnections"`
	Connections          []Connection `json:"connections"`
	ConnectionsAvailable bool         `json:"connectionsAvailable"`
	ConnectionsTruncated bool         `json:"connectionsTruncated"`
	StartedAt            *int64       `json:"startedAt"`
	MeasurementSince     *int64       `json:"measurementSince"`
	Period               string       `json:"period"`
}

type endpoint struct {
	address, secret, serverName string
	tls                         bool
}

func endpointFromConfig(config map[string]any) (endpoint, error) {
	services, _ := config["services"].([]any)
	for _, v := range services {
		s, ok := v.(map[string]any)
		if !ok || s["type"] != "api" {
			continue
		}
		host, _ := s["listen"].(string)
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return endpoint{}, errors.New("native API must listen on a literal loopback address; review its configuration")
		}
		var port int
		switch p := s["listen_port"].(type) {
		case float64:
			port = int(p)
			if p != float64(port) {
				port = 0
			}
		case int:
			port = p
		case json.Number:
			n, _ := p.Int64()
			port = int(n)
		}
		if port < 1 || port > 65535 {
			return endpoint{}, errors.New("native API listen_port is invalid")
		}
		e := endpoint{address: net.JoinHostPort(host, strconv.Itoa(port))}
		e.secret, _ = s["secret"].(string)
		if t, ok := s["tls"].(map[string]any); ok {
			e.tls, _ = t["enabled"].(bool)
			e.serverName, _ = t["server_name"].(string)
		}
		return e, nil
	}
	return endpoint{}, errors.New("Monitoring API Disabled: add a loopback native API service to a draft, validate, and apply")
}

type Monitor struct {
	config      func() (map[string]any, error)
	mu          sync.RWMutex
	latest      Snapshot
	connections map[string]Connection
	previous    *wire.Status
	previousAt  time.Time
}

func New(config func() (map[string]any, error)) *Monitor {
	return &Monitor{config: config, latest: Snapshot{Error: "Connecting to native monitoring API", Connections: []Connection{}}, connections: map[string]Connection{}}
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.latest
	s.Connections = make([]Connection, 0, len(m.connections))
	for _, c := range m.connections {
		s.Connections = append(s.Connections, c)
	}
	sort.Slice(s.Connections, func(i, j int) bool { return s.Connections[i].ID < s.Connections[j].ID })
	return s
}

func (m *Monitor) unavailable(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest = Snapshot{Error: message, Timestamp: time.Now().UnixMilli(), Connections: []Connection{}}
	m.connections = map[string]Connection{}
	m.previous = nil
}

// Run maintains one shared subscription independent of the number of browsers.
func (m *Monitor) Run(ctx context.Context) {
	for ctx.Err() == nil {
		config, err := m.config()
		var e endpoint
		if err == nil {
			e, err = endpointFromConfig(config)
		}
		if err != nil {
			m.unavailable(err.Error())
		} else {
			if err = m.subscribe(ctx, e); err != nil && ctx.Err() == nil {
				m.unavailable("Native monitoring disconnected; reconnecting. Check the API listener, credentials, and service state.")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Monitor) subscribe(parent context.Context, e endpoint) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	tc := insecure.NewCredentials()
	if e.tls {
		tc = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: e.serverName})
	}
	conn, err := grpc.NewClient("passthrough:///"+e.address, grpc.WithTransportCredentials(tc), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(4<<20)))
	if err != nil {
		return err
	}
	var readers sync.WaitGroup
	defer func() { cancel(); _ = conn.Close(); readers.Wait() }()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+e.secret)
	client := wire.NewStartedServiceClient(conn)
	checkCtx, checkCancel := context.WithTimeout(ctx, 4*time.Second)
	version, err := client.GetVersion(checkCtx, &emptypb.Empty{})
	if err != nil {
		checkCancel()
		return err
	}
	started, _ := client.GetStartedAt(checkCtx, &emptypb.Empty{})
	checkCancel()
	m.mu.Lock()
	m.previous = nil
	m.connections = map[string]Connection{}
	m.latest = Snapshot{Version: version.Version, Period: "Since sing-box process start", Timestamp: time.Now().UnixMilli()}
	if started != nil && started.StartedAt > 0 {
		v := started.StartedAt
		m.latest.StartedAt = &v
		m.latest.MeasurementSince = &v
	}
	m.mu.Unlock()
	// Re-read configuration to pick up external API address/credential changes.
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c, err := m.config()
				if err != nil {
					cancel()
					return
				}
				next, err := endpointFromConfig(c)
				if err != nil || next != e {
					cancel()
					return
				}
			}
		}
	}()
	readers.Add(1)
	go func() { defer readers.Done(); m.readConnections(ctx, client) }()
	stream, err := client.SubscribeStatus(ctx, &wire.SubscribeStatusRequest{Interval: int64(time.Second)})
	if err != nil {
		return err
	}
	// A silent server must not leave a stale "live" sample on the dashboard.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.mu.RLock()
				last := m.latest.Timestamp
				m.mu.RUnlock()
				if time.Now().UnixMilli()-last > 5000 {
					cancel()
					return
				}
			}
		}
	}()
	for {
		s, err := stream.Recv()
		if err != nil {
			return err
		}
		m.acceptStatus(s, time.Now())
	}
}

func (m *Monitor) acceptStatus(s *wire.Status, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latest.Timestamp = now.UnixMilli()
	m.latest.Available = s.TrafficAvailable
	if !s.TrafficAvailable {
		m.latest.Error = "Native API does not expose traffic counters for this instance"
		m.latest.UploadRate = nil
		m.latest.DownloadRate = nil
		m.latest.UploadTotal = nil
		m.latest.DownloadTotal = nil
		m.latest.ActiveConnections = nil
		m.previous = nil
		return
	}
	m.latest.Error = ""
	up, down, active := s.UplinkTotal, s.DownlinkTotal, s.ConnectionsIn
	m.latest.UploadTotal = &up
	m.latest.DownloadTotal = &down
	m.latest.ActiveConnections = &active
	m.latest.UploadRate = nil
	m.latest.DownloadRate = nil
	if m.previous != nil {
		elapsed := now.Sub(m.previousAt).Seconds()
		if up >= m.previous.UplinkTotal && down >= m.previous.DownlinkTotal && elapsed > 0 && elapsed < 5 {
			ur := float64(up-m.previous.UplinkTotal) / elapsed
			dr := float64(down-m.previous.DownlinkTotal) / elapsed
			m.latest.UploadRate = &ur
			m.latest.DownloadRate = &dr
		} else if up < m.previous.UplinkTotal || down < m.previous.DownlinkTotal {
			boundary := now.UnixMilli()
			m.latest.MeasurementSince = &boundary
			m.latest.Period = "Since observed counter reset"
		}
	}
	m.previous = s
	m.previousAt = now
}

func (m *Monitor) readConnections(ctx context.Context, client wire.StartedServiceClient) {
	stream, err := client.SubscribeConnections(ctx, &wire.SubscribeConnectionsRequest{Interval: int64(time.Second)})
	if err != nil {
		return
	}
	defer func() {
		m.mu.Lock()
		m.latest.ConnectionsAvailable = false
		m.connections = map[string]Connection{}
		m.mu.Unlock()
	}()
	for {
		events, err := stream.Recv()
		if err != nil {
			return
		}
		m.mu.Lock()
		m.latest.ConnectionsAvailable = true
		if events.Reset_ {
			m.connections = map[string]Connection{}
			m.latest.ConnectionsTruncated = false
		}
		for _, ev := range events.Events {
			if ev.Type == wire.ConnectionEventType_CONNECTION_EVENT_CLOSED {
				delete(m.connections, ev.Id)
				continue
			}
			if c := ev.Connection; c != nil {
				if c.ClosedAt > 0 {
					delete(m.connections, ev.Id)
					continue
				}
				if _, ok := m.connections[c.Id]; !ok && len(m.connections) >= 1000 {
					m.latest.ConnectionsTruncated = true
					continue
				}
				m.connections[c.Id] = Connection{ID: c.Id, CreatedAt: c.CreatedAt, Inbound: c.Inbound, Outbound: c.Outbound, Network: c.Network, Source: c.Source, Destination: c.Destination, Domain: c.Domain, Protocol: c.Protocol, UploadTotal: c.UplinkTotal, DownloadTotal: c.DownlinkTotal}
			} else if c, ok := m.connections[ev.Id]; ok {
				c.UploadTotal += ev.UplinkDelta
				c.DownloadTotal += ev.DownlinkDelta
				m.connections[ev.Id] = c
			}
		}
		m.mu.Unlock()
	}
}

func (m *Monitor) Handler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unavailable", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
		data, _ := json.Marshal(m.Snapshot())
		if _, err := fmt.Fprintf(w, "event: telemetry\ndata: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

// Check verifies a configured loopback native API after deployment. Absence of
// native monitoring is valid and does not prevent configuration management.
func Check(ctx context.Context, config map[string]any) error {
	services, _ := config["services"].([]any)
	found := false
	for _, v := range services {
		if s, ok := v.(map[string]any); ok && s["type"] == "api" {
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	e, err := endpointFromConfig(config)
	if err != nil {
		return err
	}
	tc := insecure.NewCredentials()
	if e.tls {
		tc = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: e.serverName})
	}
	conn, err := grpc.NewClient("passthrough:///"+e.address, grpc.WithTransportCredentials(tc))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+e.secret)
	_, err = wire.NewStartedServiceClient(conn).GetVersion(ctx, &emptypb.Empty{})
	if err != nil {
		return errors.New("native API health check failed: listener or authentication unavailable")
	}
	return nil
}
