package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompressedAssets(t *testing.T) {
	h := Handler()
	call := func(path, encoding string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Accept-Encoding", encoding)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	plain := call("/", "")
	if plain.Code == 404 {
		t.Skip("frontend build required for asset tests")
	}
	compressed := call("/", "gzip")
	reader, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(data, plain.Body.Bytes()) {
		t.Fatal("gzip representation differs")
	}
	if compressed.Header().Get("Vary") != "Accept-Encoding" || compressed.Header().Get("Cache-Control") != "no-cache" {
		t.Fatal("incorrect caching")
	}
	if call("/", "gzip;q=0").Header().Get("Content-Encoding") != "" {
		t.Fatal("ignored gzip opt-out")
	}
	if call("/assets/missing.js", "gzip").Code != 404 {
		t.Fatal("missing script returned HTML")
	}
	if call("/", "gzip").Code != 200 {
		t.Fatal("cache reuse failed")
	}
}

func TestSchemaCompressionScope(t *testing.T) {
	for _, tc := range []struct {
		path       string
		status     int
		compressed bool
	}{
		{"/api/schema", 200, true}, {"/api/schema", 401, false}, {"/api/config", 200, false}, {"/api/telemetry", 200, false},
	} {
		h := SchemaCompression(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(`{"fixture":true}`))
		}))
		r := httptest.NewRequest("GET", tc.path, nil)
		r.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if (w.Header().Get("Content-Encoding") == "gzip") != tc.compressed {
			t.Fatal("incorrect compression scope", tc.path, tc.status)
		}
		if tc.compressed {
			reader, err := gzip.NewReader(w.Body)
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(reader)
			if string(data) != `{"fixture":true}` {
				t.Fatal("incorrect schema")
			}
		}
	}
}
