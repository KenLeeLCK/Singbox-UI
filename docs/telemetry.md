# Native monitoring protocol

The adapter uses the installed sing-box native `api` service, introduced in 1.14. It does not use VPS network-interface counters or invent samples. The official [API service reference](https://sing-box.sagernet.org/configuration/service/api/) specifies gRPC and bearer authentication.

The read-only protobuf subset in `backend/internal/telemetry/wire/monitor.proto` preserves the package, methods, message names, field types and numbers from [sing-box v1.14.4](https://github.com/SagerNet/sing-box/blob/v1.14.4/daemon/started_service.proto). The original protocol is GPL-3.0-or-later; attribution is retained. No control RPC is exposed by this adapter.

Verified against the upstream server implementation:

- `GetVersion` detects connectivity and API version.
- `GetStartedAt` returns milliseconds since Unix epoch.
- `SubscribeStatus.interval` uses nanoseconds; Manager requests one second.
- Native `uplinkTotal` / `downlinkTotal` are cumulative tracked proxy bytes since the sing-box process began. Rates are computed from the difference between actual counters divided by actual elapsed time. The first sample, counter reset, and long sample gaps have null rates. Totals come directly from the server, never from integrating a partial chart.
- `SubscribeConnections` sends an initial reset and subsequent new/update/closed events. The Manager retains at most 1,000 connection details and marks truncation. Active connection counts come from the native status stream independently.
- Missing tracking, authentication failure, disconnected or silent streams display unavailable. The shared subscriber retries after two seconds and rereads live configuration to reconnect after external listener/credential changes.

Only literal loopback listeners are accepted. TLS API listeners use normal certificate validation; custom trust roots are not currently configurable. API credentials remain server-side for polling, although an authenticated administrator can view them in the raw sing-box configuration editor.

## Executed verification

On macOS ARM64, the official v1.14.4 archive was verified against its GitHub release SHA256 digest and executed. Generated schema: `.tools/schema.json` (development artifact, not committed).

```sh
cd backend
SING_BOX_BIN="$PWD/../.tools/sing-box-1.14.4-darwin-arm64/sing-box" go test -race -v ./internal/telemetry/...
```

Passed: real native API connection, loopback HTTP proxy transfer of 256 KiB, actual cumulative download 263,189 bytes and upload 115 bytes, connection subscription, counter reset/gap handling, rejection of non-loopback destinations. Test configurations use ephemeral loopback ports and temporary directories. No host service was altered.

Regenerate bindings with protoc and pinned `protoc-gen-go@v1.36.10`, `protoc-gen-go-grpc@v1.5.1` if the protocol subset changes. Installed sing-box schemas, not the checked-in protobuf version, determine editing features.
