package telemetry

import (
	"github.com/sing-box-manager/sing-box-manager/backend/internal/telemetry/wire"
	"testing"
	"time"
)

func TestCounterRatesResetAndMissingSamples(t *testing.T) {
	m := New(nil)
	now := time.Now()
	m.acceptStatus(&wire.Status{TrafficAvailable: true, UplinkTotal: 100, DownlinkTotal: 200}, now)
	if m.Snapshot().UploadRate != nil {
		t.Fatal("first sample must not invent a rate")
	}
	m.acceptStatus(&wire.Status{TrafficAvailable: true, UplinkTotal: 300, DownlinkTotal: 600}, now.Add(2*time.Second))
	if got := *m.Snapshot().UploadRate; got != 100 {
		t.Fatalf("rate=%v", got)
	}
	m.acceptStatus(&wire.Status{TrafficAvailable: true, UplinkTotal: 1, DownlinkTotal: 2}, now.Add(3*time.Second))
	if m.Snapshot().UploadRate != nil || m.Snapshot().MeasurementSince == nil {
		t.Fatal("counter reset not represented")
	}
	m.acceptStatus(&wire.Status{TrafficAvailable: true, UplinkTotal: 10, DownlinkTotal: 20}, now.Add(20*time.Second))
	if m.Snapshot().UploadRate != nil {
		t.Fatal("gap invented a rate")
	}
	m.acceptStatus(&wire.Status{}, now.Add(21*time.Second))
	if m.Snapshot().Available || m.Snapshot().UploadTotal != nil {
		t.Fatal("unavailable counter exposed")
	}
}
func TestLoopbackOnly(t *testing.T) {
	for _, host := range []string{"0.0.0.0", "example.com", "169.254.169.254", ""} {
		_, err := endpointFromConfig(map[string]any{"services": []any{map[string]any{"type": "api", "listen": host, "listen_port": float64(9090)}}})
		if err == nil {
			t.Fatalf("accepted %q", host)
		}
	}
	for _, host := range []string{"127.0.0.1", "::1"} {
		_, err := endpointFromConfig(map[string]any{"services": []any{map[string]any{"type": "api", "listen": host, "listen_port": float64(9090)}}})
		if err != nil {
			t.Fatal(err)
		}
	}
}
