# HTTP API

All routes use the same origin as the embedded UI. Responses and streams are `no-store`. Error responses may be JSON `{ "error": "..." }` or plain text for authentication/CSRF failures. JSON bodies are limited to 4 MiB. API endpoints other than login require a valid `manager_session` cookie.

For browser writes, send a matching `Origin`. For non-browser clients send `X-Requested-With: sing-box-manager` and retain the login cookie. Never place the token in a URL. Cross-site Fetch requests and mismatched origins are denied.

| Method | Path | Request / response |
| --- | --- | --- |
| GET | `/healthz` | Public minimal process health `{ok:true}`; not proxy-health proof |
| POST | `/api/login` | `{token:string}` → session cookie; rate limited |
| POST | `/api/logout` | Invalidate current session |
| GET | `/api/status` | Installed version, unit, systemd state, start timestamp/local/read-only flags |
| GET | `/api/config` | `{config,hash,sources,readOnly,draft?:{config,baseHash}}` |
| GET | `/api/schema` | Installed binary's JSON Schema document; explicit error if unsupported |
| PUT | `/api/draft` | `{config:object,baseHash:string}`; preserve live file, reject stale hash |
| POST | `/api/validate` | Check persisted draft using application validation and installed binary |
| POST | `/api/apply` | Revalidate, backup, replace, restart, health check, commit or rollback |
| GET | `/api/history` | `[{id,created}]` backup identifiers |
| POST | `/api/restore` | `{id:string}` → copy approved history identifier into a draft |
| POST | `/api/monitoring/prepare` | Generate loopback native API configuration in a draft, no immediate restart |
| GET | `/api/telemetry` | Named SSE `telemetry` events, one per second |
| GET | `/api/logs` | SSE messages `{line:string}`; named error event with reason when unavailable |

Telemetry snapshot fields:

```ts
interface Telemetry {
  available: boolean;
  error?: string;
  timestamp: number; // Unix milliseconds
  version?: string;
  uploadRate: number | null; // bytes/second
  downloadRate: number | null;
  uploadTotal: number | null; // native cumulative counters
  downloadTotal: number | null;
  activeConnections: number | null;
  connectionsAvailable: boolean;
  connectionsTruncated: boolean;
  connections: Array<{
    id: string; createdAt: number; // native connection creation timestamp
    inbound: string; outbound: string;
    network: string; source: string; destination: string;
    domain: string; protocol: string;
    uploadTotal: number; downloadTotal: number;
  }>;
  startedAt: number | null; // Unix milliseconds
  measurementSince: number | null;
  period: string;
}
```

Null means unavailable, including initial/gapped rates. Do not convert missing counters to zero or label a disconnected snapshot live. The connection count is independent from the bounded detail list.

The helper uses the same fixed management route implementations over its restricted Unix socket; it does not expose login or a network listener. There is no arbitrary file-read, arbitrary command, unit-control or remote-proxy route.
