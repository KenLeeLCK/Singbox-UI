package web

import (
	"bytes"
	"compress/gzip"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Handler serves embedded assets with cached gzip representations. API and SSE
// responses never pass through this handler. Unknown asset paths must be 404s,
// not HTML disguised as a successful JavaScript response.
func Handler() http.Handler {
	assets, _ := fs.Sub(Assets, "dist")
	var compressed sync.Map
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." || name == "" {
			name = "index.html"
		}
		data, err := fs.ReadFile(assets, name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; worker-src 'self' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(name)))
		w.Header().Set("Vary", "Accept-Encoding")
		if name == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			cached, ok := compressed.Load(name)
			if !ok {
				var buffer bytes.Buffer
				writer := gzip.NewWriter(&buffer)
				_, _ = writer.Write(data)
				_ = writer.Close()
				cached, _ = compressed.LoadOrStore(name, buffer.Bytes())
			}
			data = cached.([]byte)
			w.Header().Set("Content-Encoding", "gzip")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}
func acceptsGzip(value string) bool {
	for _, part := range strings.Split(value, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		if fields[0] != "gzip" {
			continue
		}
		for _, option := range fields[1:] {
			k, v, ok := strings.Cut(strings.TrimSpace(option), "=")
			if ok && k == "q" {
				q, err := strconv.ParseFloat(v, 64)
				return err == nil && q > 0 && q <= 1
			}
		}
		return true
	}
	return false
}

// SchemaCompression compresses only the public schema shape after normal API
// authorization. Configuration, credentials, mutations and SSE are excluded.
func SchemaCompression(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/schema" || r.Method != http.MethodGet || !acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		writer := &schemaWriter{ResponseWriter: w}
		defer func() {
			if writer.gzip != nil {
				writer.gzip.Close()
			}
		}()
		next.ServeHTTP(writer, r)
	})
}

type schemaWriter struct {
	http.ResponseWriter
	gzip        *gzip.Writer
	wroteHeader bool
}

func (w *schemaWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if status == http.StatusOK {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
		w.gzip = gzip.NewWriter(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *schemaWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.gzip != nil {
		return w.gzip.Write(b)
	}
	return w.ResponseWriter.Write(b)
}
