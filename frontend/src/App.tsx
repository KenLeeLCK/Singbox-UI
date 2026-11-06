import { lazy, Suspense, useEffect, useRef, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
const Editor = lazy(() => import("./components/CodeEditor"));
const DiffEditor = lazy(() =>
  import("./components/CodeEditor").then((m) => ({ default: m.DiffEditor })),
);
import {
  Activity,
  ArrowDownLeft,
  ArrowUpRight,
  ArrowRight,
  Box,
  Check,
  ChevronDown,
  Code2,
  Copy,
  FileJson,
  Gauge,
  GitBranch,
  Layers,
  Lock,
  LogOut,
  Plus,
  Radio,
  RefreshCw,
  Search,
  Settings,
  Shield,
  Sun,
  Terminal,
  Trash2,
  X,
  GripVertical,
} from "lucide-react";
import {
  api,
  ApiError,
  bytes,
  collection,
  dependency,
  objectSchema,
  tagNames,
  matchSchema,
  Obj,
  pretty,
  replaceCollection,
  types,
  resolve,
  variants,
} from "./lib/model";
import { SchemaForm } from "./components/SchemaForm";
import { Chart } from "./components/Chart";
import { Logs } from "./components/Logs";
const pages = [
  ["Overview", Gauge],
  ["Inbounds", ArrowDownLeft],
  ["Outbounds", ArrowUpRight],
  ["Routing", GitBranch],
  ["Rule Sets", Layers],
  ["JSON Editor", Code2],
  ["Settings", Settings],
] as const;
function Modal({
  title,
  children,
  onClose,
  wide = false,
}: {
  title: string;
  children: React.ReactNode;
  onClose: () => void;
  wide?: boolean;
}) {
  return (
    <Dialog.Root open onOpenChange={(o) => !o && onClose()}>
      <Dialog.Portal>
        <Dialog.Overlay className="overlay" />
        <Dialog.Content
          className={"modal glass " + (wide ? "wide-modal" : "")}
          aria-describedby={undefined}
        >
          <div className="modal-title">
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Close aria-label="Close">
              <X size={19} />
            </Dialog.Close>
          </div>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
export default function App() {
  const [page, setPage] = useState("Overview"),
    [config, setConfig] = useState<Obj>({}),
    [live, setLive] = useState<Obj>({}),
    [base, setBase] = useState(""),
    [schema, setSchema] = useState<Obj>({}),
    [status, setStatus] = useState<Obj>({}),
    [info, setInfo] = useState<Obj>({}),
    [telemetry, setTelemetry] = useState<Obj>({ available: false }),
    [samples, setSamples] = useState<Obj[]>([]),
    [minutes, setMinutes] = useState(1),
    [auth, setAuth] = useState(false),
    [token, setToken] = useState(""),
    [loading, setLoading] = useState(true),
    [ready, setReady] = useState(false),
    [message, setMessage] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false),
    [logs, setLogs] = useState(false),
    [diff, setDiff] = useState(false),
    [query, setQuery] = useState(""),
    [sort, setSort] = useState("tag"),
    [editing, setEditing] = useState<{
      section: string;
      index: number;
      value: Obj;
    } | null>(null),
    [raw, setRaw] = useState(""),
    [rawMode, setRawMode] = useState(false),
    [history, setHistory] = useState<Obj[]>([]),
    [json, setJson] = useState("{}"),
    [light, setLight] = useState(false),
    [conflict, setConflict] = useState(false);
  const dirty = pretty(config) !== pretty(live);
  useEffect(() => {
    document.documentElement.dataset.theme = light ? "light" : "dark";
  }, [light]);
  const draftRef = useRef(dirty);
  draftRef.current = dirty;
  const baseRef = useRef(base);
  baseRef.current = base;
  const loadGeneration = useRef(0);
  function handleError(e: unknown) {
    setError(
      e instanceof Error ? e.message : "Unexpected error. Please retry.",
    );
    if (e instanceof ApiError && e.status === 401) {
      loadGeneration.current++;
      setAuth(true);
      setReady(false);
      setLogs(false);
      setDiff(false);
      setEditing(null);
    }
  }
  async function load() {
    const generation = ++loadGeneration.current;
    setLoading(true);
    setError("");
    try {
      const c = await api("config");
      if (!c?.config || typeof c.config !== "object" || Array.isArray(c.config))
        throw new Error(
          "Configuration could not be loaded. Check the Manager helper and retry.",
        );
      if (generation !== loadGeneration.current) return;
      setInfo(c);
      setLive(c.config);
      setConfig(c.draft?.config || c.config);
      setBase(c.draft?.baseHash || c.hash);
      setConflict(!!c.draft && c.draft.baseHash !== c.hash);
      setJson(pretty(c.draft?.config || c.config));
      setAuth(false);
      setReady(true);
      setLoading(false);
      const optional = await Promise.allSettled([
        api("status"),
        api("schema"),
        api("history"),
      ]);
      if (generation !== loadGeneration.current) return;
      for (const result of optional) {
        if (
          result.status === "rejected" &&
          result.reason instanceof ApiError &&
          result.reason.status === 401
        )
          throw result.reason;
      }
      const [st, sc, hi] = optional;
      setStatus(st.status === "fulfilled" ? st.value || {} : {});
      setSchema(sc.status === "fulfilled" ? sc.value || {} : {});
      const h = hi.status === "fulfilled" ? hi.value : null;
      setHistory(Array.isArray(h) ? h : h?.history || []);
      if (optional.some((r) => r.status === "rejected"))
        setError(
          "Some installation details are unavailable. Refresh configuration to retry; JSON editing remains available.",
        );
    } catch (e) {
      if (generation === loadGeneration.current) {
        handleError(e);
        setLoading(false);
      }
    } finally {
      if (generation === loadGeneration.current) setLoading(false);
    }
  }
  useEffect(() => {
    void load();
    return () => {
      loadGeneration.current++;
    };
  }, []);
  useEffect(() => {
    if (auth || loading || !ready) return;
    let active = true;
    let polling = false;
    const es = new EventSource("/api/telemetry");
    const receive = (e: MessageEvent) => {
      try {
        const d = JSON.parse(e.data);
        setTelemetry(d);
        setSamples((s) =>
          [...s, { ...d, time: Date.now() / 1000 }].slice(-900),
        );
      } catch {
        /* Ignore malformed frames */
      }
    };
    es.addEventListener("telemetry", receive);
    es.onmessage = receive;
    es.onerror = () =>
      setTelemetry((t) => ({
        ...t,
        available: false,
        error: "Monitoring connection lost. Reconnecting…",
      }));
    const timer = setInterval(() => {
      if (polling) return;
      polling = true;
      const report = (e: unknown) => {
        if (active) handleError(e);
      };
      void Promise.all([
        api("status")
          .then((s) => {
            if (active) setStatus(s);
          })
          .catch(report),
        api("config")
          .then((c) => {
            if (!active) return;
            if (c.hash !== baseRef.current) {
              if (draftRef.current) {
                setLive(c.config);
                setConflict(true);
              } else {
                setLive(c.config);
                setConfig(c.config);
                setJson(pretty(c.config));
                setBase(c.hash);
                setInfo(c);
              }
            }
          })
          .catch(report),
      ]).finally(() => {
        polling = false;
      });
    }, 5000);
    return () => {
      active = false;
      es.close();
      clearInterval(timer);
    };
  }, [auth, loading, ready]);
  const change = (c: Obj) => {
    setConfig(c);
    setJson(pretty(c));
  };
  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await fn();
    } catch (e) {
      handleError(e);
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    if (pretty(JSON.parse(json)) !== pretty(config))
      throw Error("Synchronize the JSON editor before saving");
    await api("draft", "PUT", { config, baseHash: base });
    setMessage("Draft saved. The running service is unchanged.");
  }
  async function validate() {
    await save();
    await api("validate", "POST", {});
    setMessage(
      "Installed sing-box validation passed. Review the diff before applying.",
    );
  }
  const tags = (config.outbounds || []).flatMap(tagNames);
  const section =
    page === "Inbounds"
      ? "inbounds"
      : page === "Outbounds"
        ? "outbounds"
        : page === "Routing"
          ? "rules"
          : "rule_set";
  const items = collection(config, section);
  const openEdit = (index: number, value: Obj) => {
    setError("");
    setEditing({ section, index, value: structuredClone(value) });
    setRaw(pretty(value));
    setRawMode(false);
  };
  const remove = (index: number) => {
    const item = items[index];
    if (tagNames(item).some((tag) => dependency(config, tag))) {
      setError(`Resolve references to “${item.tag}” before deleting it.`);
      return;
    }
    change(
      replaceCollection(
        config,
        section,
        items.filter((_, i) => i !== index),
      ),
    );
  };
  if (auth)
    return (
      <div className="login glass">
        <div className="brand-icon">
          <Box />
        </div>
        <h1>
          Your network.
          <br />
          Under your control.
        </h1>
        <p>
          Sign in to sing-box Manager with the access token from your local
          installation.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            run(async () => {
              await api("login", "POST", { token });
              setToken("");
              setPage("Overview");
              setAuth(false);
              setReady(false);
              await load();
            });
          }}
        >
          <label className="field">
            <span>Manager access token</span>
            <input
              autoFocus
              type="password"
              autoComplete="current-password"
              required
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
          </label>
          <button className="primary" disabled={busy}>
            Unlock Manager <ArrowRight size={16} />
          </button>
        </form>
        {error && <p className="error">{error}</p>}
        <small>
          <Lock size={12} /> Private access · token stays on this device
        </small>
      </div>
    );
  if (!ready)
    return (
      <div className="login glass">
        <Box />
        <h1>
          {loading
            ? "Loading your installation"
            : "Unable to load configuration"}
        </h1>
        {error && (
          <p className="error" role="alert">
            {error}
          </p>
        )}
        <p>Check Manager connectivity and the helper service, then retry.</p>
        <button
          className="primary"
          disabled={loading}
          onClick={() => void load()}
        >
          <RefreshCw size={16} /> Retry loading
        </button>
      </div>
    );
  return (
    <div className={light ? "app light" : "app"}>
      <aside className="sidebar glass">
        <a
          className="brand"
          href="#"
          onClick={(e) => {
            e.preventDefault();
            setPage("Overview");
          }}
        >
          <span className="brand-icon">
            <Box size={23} />
          </span>
          <span>
            sing-box<small>MANAGER</small>
          </span>
        </a>
        <div className="workspace">
          <span className="server-dot" />
          <div>
            Local instance
            <small>
              {status.version?.split("\n")[0] || "Detecting installation"}
            </small>
          </div>
          <ChevronDown size={14} />
        </div>
        <span className="nav-label">WORKSPACE</span>
        <nav>
          {pages.map(([label, Icon]) => (
            <button
              key={label}
              className={page === label ? "active" : ""}
              onClick={() => {
                setPage(label);
                setQuery("");
              }}
            >
              <Icon size={18} />
              <span>{label}</span>
              {label === "Inbounds" && (
                <small>{config.inbounds?.length || 0}</small>
              )}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="private">
            <Shield size={17} />
            <span>
              Private by design<small>Localhost · secured access</small>
            </span>
          </div>
          <button onClick={() => setLight(!light)}>
            <Sun size={17} />
            {light ? "Dark appearance" : "Light appearance"}
          </button>
        </div>
      </aside>
      <div className="main">
        <header>
          <div className="breadcrumb">
            Workspace <span>/</span> <strong>{page}</strong>
          </div>
          <div className="header-actions">
            <span className="connection">
              <i className={status.running ? "green" : ""} />
              {status.running
                ? "Service running"
                : status.local
                  ? "Local inspection"
                  : status.state || "Service unavailable"}
            </span>
            <button onClick={() => setLogs(true)}>
              <Terminal size={16} /> Live Logs
            </button>
            <button
              className="icon-button"
              aria-label="Refresh configuration"
              disabled={dirty || busy}
              onClick={() => run(load)}
            >
              <RefreshCw size={17} />
            </button>
          </div>
        </header>
        <main>
          <div className="page-heading">
            <div className="eyebrow">YOUR NETWORK, AT A GLANCE</div>
            <div className="heading-row">
              <div>
                <h1>{page === "Overview" ? "A clearer view." : page}</h1>
                <p>
                  {page === "Overview"
                    ? "Everything flowing through your sing-box, in one place."
                    : page === "Routing"
                      ? "Give every connection the right direction."
                      : page === "JSON Editor"
                        ? "Native configuration. Complete control."
                        : "Thoughtfully configured. Safely deployed."}
                </p>
              </div>
              {page === "Overview" ? (
                <div className="segmented">
                  {[1, 5, 15].map((m) => (
                    <button
                      className={minutes === m ? "selected" : ""}
                      onClick={() => setMinutes(m)}
                      key={m}
                    >
                      {m}m
                    </button>
                  ))}
                </div>
              ) : (
                !["Settings", "JSON Editor"].includes(page) && (
                  <button
                    className="primary"
                    disabled={info.readOnly || !Object.keys(schema).length}
                    onClick={() => {
                      const ts = types(schema, section);
                      openEdit(
                        -1,
                        section === "rules"
                          ? { action: "route" }
                          : { type: ts[0] || "", tag: "" },
                      );
                    }}
                  >
                    <Plus size={16} /> Add{" "}
                    {page === "Routing"
                      ? "rule"
                      : page === "Rule Sets"
                        ? "rule set"
                        : section.slice(0, -1)}
                  </button>
                )
              )}
            </div>
          </div>
          {error && (
            <div className="notice error" role="alert">
              {error}
              <button onClick={() => setError("")} aria-label="Dismiss error">
                <X size={15} />
              </button>
            </div>
          )}
          {message && (
            <div className="notice success" role="status">
              <Check size={16} />
              {message}
            </div>
          )}
          {conflict && (
            <div className="notice warning">
              The live file changed outside Manager. Compare your draft and
              reload before saving.
              <button onClick={() => setDiff(true)}>Compare</button>
              <button
                onClick={() =>
                  run(async () => {
                    const c = await api("config");
                    await api("draft", "PUT", {
                      config: c.config,
                      baseHash: c.hash,
                    });
                    await load();
                    setConflict(false);
                  })
                }
              >
                Discard draft & use live
              </button>
            </div>
          )}
          {info.readOnly && (
            <div className="notice warning">
              This installation is read-only. Multiple files or unsupported
              service arguments require file-level editing outside Manager.
            </div>
          )}
          {loading ? (
            <div className="empty glass">
              <RefreshCw className="spin" />
              <h3>Discovering your installation</h3>
            </div>
          ) : (
            <>
              {page === "Overview" && (
                <>
                  <div className="metric-grid">
                    {[
                      ["Upload traffic", "uploadRate", ArrowUpRight, "#9cc3ff"],
                      [
                        "Download traffic",
                        "downloadRate",
                        ArrowDownLeft,
                        "#b5a2ff",
                      ],
                    ].map(([title, field, Icon, color]) => {
                      const I = Icon as typeof ArrowUpRight;
                      return (
                        <section className="glass metric" key={title as string}>
                          <div className="card-title">
                            <span
                              className="metric-icon"
                              style={{ color: color as string }}
                            >
                              <I size={18} />
                            </span>
                            {title as string}
                            <span className="spacer" />
                            <span className="live-label">
                              {telemetry.available ? "● LIVE" : "○ OFFLINE"}
                            </span>
                          </div>
                          <div className="metric-value">
                            {bytes(
                              telemetry.available
                                ? telemetry[field as string]
                                : null,
                            )}
                            <span>/s</span>
                          </div>
                          <Chart
                            samples={samples}
                            field={field as string}
                            color={color as string}
                            minutes={minutes}
                          />
                          <div className="chart-caption">
                            <span>{minutes} minute window</span>
                            <span>Now</span>
                          </div>
                        </section>
                      );
                    })}
                    <section className="glass metric compact">
                      <div className="card-title">
                        <Activity size={18} /> Active connections
                      </div>
                      <div className="metric-value">
                        {telemetry.available
                          ? (telemetry.activeConnections ?? "—")
                          : "—"}
                      </div>
                      <p>Connections handled by sing-box</p>
                      <div className="mini-footer">
                        <Radio size={15} />{" "}
                        {telemetry.available
                          ? "Native API connected"
                          : "Awaiting monitoring API"}
                      </div>
                    </section>
                    <section className="glass metric compact">
                      <div className="card-title">
                        <Layers size={18} /> Total traffic
                      </div>
                      <div className="totals">
                        <div>
                          <ArrowUpRight size={16} />
                          <span>Uploaded</span>
                          <strong>
                            {bytes(
                              telemetry.available
                                ? telemetry.uploadTotal
                                : null,
                            )}
                          </strong>
                        </div>
                        <div>
                          <ArrowDownLeft size={16} />
                          <span>Downloaded</span>
                          <strong>
                            {bytes(
                              telemetry.available
                                ? telemetry.downloadTotal
                                : null,
                            )}
                          </strong>
                        </div>
                      </div>
                      <div className="mini-footer">
                        {telemetry.measurementSince
                          ? `Since ${new Date(telemetry.measurementSince).toLocaleString()}`
                          : "Totals unavailable until API supplies counters"}
                      </div>
                    </section>
                  </div>
                  {!telemetry.available && (
                    <section className="monitor-banner glass">
                      <div className="banner-icon">
                        <Radio size={24} />
                      </div>
                      <div>
                        <h3>Bring your network into view</h3>
                        <p>
                          {telemetry.error ||
                            "Monitoring API disabled. Enable the private native API to see real traffic and connections."}
                        </p>
                        <small>
                          Enabling monitoring creates a draft and requires a
                          service restart when applied.
                        </small>
                      </div>
                      <button
                        onClick={() =>
                          run(async () => {
                            if (dirty) await save();
                            await api("monitoring/prepare", "POST", {});
                            await load();
                            setMessage(
                              "Monitoring configuration prepared as a draft. Validate and review before applying.",
                            );
                          })
                        }
                        disabled={info.readOnly || busy}
                      >
                        Prepare monitoring <ArrowRight size={15} />
                      </button>
                    </section>
                  )}
                  <div className="overview-bottom">
                    <section className="glass panel">
                      <div className="section-title">
                        <h3>Service health</h3>
                        <span className="pill">SYSTEM</span>
                      </div>
                      <div className="health-row">
                        <span>sing-box version</span>
                        <strong>
                          {status.version?.split("\n")[0] || "Not detected"}
                        </strong>
                      </div>
                      <div className="health-row">
                        <span>Service state</span>
                        <strong>
                          {status.local
                            ? "Local inspection · unmanaged"
                            : status.state || "Unavailable"}
                        </strong>
                      </div>
                      <div className="health-row">
                        <span>Service uptime</span>
                        <strong>
                          {status.uptime ||
                            (telemetry.startedAt
                              ? `${Math.floor((Date.now() - telemetry.startedAt) / 60000)} min (API instance)`
                              : "Unavailable")}
                        </strong>
                      </div>
                      <div className="health-row">
                        <span>Management API</span>
                        <strong>
                          {telemetry.available ? "Connected" : "Unavailable"}
                        </strong>
                      </div>
                      <div className="health-row">
                        <span>Configuration</span>
                        <strong>
                          {info.readOnly
                            ? "Read-only"
                            : dirty
                              ? "Draft changes"
                              : "Synchronized"}
                        </strong>
                      </div>
                    </section>
                    <section className="glass panel">
                      <div className="section-title">
                        <h3>Your configuration</h3>
                        <Shield size={17} />
                      </div>
                      <div className="config-summary">
                        {[
                          ["Inbounds", config.inbounds?.length || 0],
                          ["Outbounds", config.outbounds?.length || 0],
                          ["Routing", config.route?.rules?.length || 0],
                        ].map(([name, n]) => (
                          <button
                            key={name}
                            onClick={() => setPage(name as string)}
                          >
                            <strong>{n}</strong>
                            <span>{name}</span>
                          </button>
                        ))}
                      </div>
                      <div className="subtle-line" />
                      <p className="muted">
                        Your native configuration remains the source of truth.
                        Every change starts as a draft.
                      </p>
                      <button
                        className="text-button"
                        onClick={() => setPage("JSON Editor")}
                      >
                        Open configuration <ArrowRight size={15} />
                      </button>
                    </section>
                  </div>
                  <section className="glass panel connections">
                    <div className="section-title">
                      <h3>Active connections</h3>
                      <span className="pill">
                        {telemetry.activeConnections ?? "—"} CONNECTIONS
                      </span>
                    </div>
                    {telemetry.connectionsTruncated && (
                      <p className="muted">
                        Showing up to 1,000 connection details; total count
                        remains native.
                      </p>
                    )}
                    {telemetry.connections?.length ? (
                      <div className="table-wrap">
                        <table>
                          <thead>
                            <tr>
                              <th>Destination</th>
                              <th>Inbound</th>
                              <th>Outbound</th>
                              <th>Network</th>
                            </tr>
                          </thead>
                          <tbody>
                            {telemetry.connections.map((c: Obj, i: number) => (
                              <tr key={c.id || i}>
                                <td>
                                  {c.destination ||
                                    c.host ||
                                    c.metadata?.host ||
                                    "—"}
                                </td>
                                <td>
                                  {c.inbound || c.metadata?.inboundName || "—"}
                                </td>
                                <td>
                                  {c.outbound || c.chains?.join(" → ") || "—"}
                                </td>
                                <td>
                                  {c.network || c.metadata?.network || "—"}
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    ) : (
                      <div className="empty-inline">
                        <Activity size={22} />
                        <span>
                          {telemetry.available
                            ? telemetry.connectionsAvailable
                              ? "No active connections"
                              : "Connection details unavailable; traffic statistics remain available."
                            : "Connection details will appear when monitoring is available."}
                        </span>
                      </div>
                    )}
                  </section>
                </>
              )}
              {["Inbounds", "Outbounds", "Routing", "Rule Sets"].includes(
                page,
              ) && (
                <section className="glass panel">
                  <div className="toolbar">
                    <div className="search">
                      <Search size={16} />
                      <input
                        aria-label="Search configuration"
                        placeholder="Search by tag, protocol, or address…"
                        value={query}
                        onChange={(e) => setQuery(e.target.value)}
                      />
                    </div>
                    <span className="pill">{items.length} ITEMS</span>
                    {section !== "rules" && (
                      <select
                        aria-label="Sort entries"
                        value={sort}
                        onChange={(e) => setSort(e.target.value)}
                      >
                        <option value="tag">Sort by tag</option>
                        <option value="type">Sort by protocol</option>
                        <option value="listen_port">Sort by port</option>
                      </select>
                    )}
                  </div>
                  {section === "rules" && (
                    <div className="health-row">
                      <span>Default outbound</span>
                      <select
                        aria-label="Default outbound"
                        disabled={info.readOnly}
                        value={config.route?.final || ""}
                        onChange={(e) =>
                          change({
                            ...config,
                            route: { ...config.route, final: e.target.value },
                          })
                        }
                      >
                        <option value="">sing-box default</option>
                        {tags.map((t: string) => (
                          <option key={t}>{t}</option>
                        ))}
                      </select>
                    </div>
                  )}
                  {items.length ? (
                    <div className="node-list">
                      {items
                        .map((value, index) => ({ value, index }))
                        .filter(({ value }) =>
                          pretty(value)
                            .toLowerCase()
                            .includes(query.toLowerCase()),
                        )
                        .sort((a, b) =>
                          section === "rules"
                            ? a.index - b.index
                            : String(a.value[sort] || "").localeCompare(
                                String(b.value[sort] || ""),
                                undefined,
                                { numeric: true },
                              ),
                        )
                        .map(({ value: v, index: i }) => (
                          <div
                            className="node-row"
                            key={i}
                            draggable={section === "rules" && !info.readOnly}
                            onDragStart={(e) =>
                              e.dataTransfer.setData("text/plain", String(i))
                            }
                            onDragOver={(e) => e.preventDefault()}
                            onDrop={(e) => {
                              e.preventDefault();
                              const from = Number(
                                e.dataTransfer.getData("text/plain"),
                              );
                              if (
                                !Number.isInteger(from) ||
                                from < 0 ||
                                from >= items.length
                              )
                                return;
                              const list = [...items];
                              list.splice(i, 0, list.splice(from, 1)[0]);
                              change(replaceCollection(config, section, list));
                            }}
                          >
                            <div className="node-symbol">
                              {section === "rules" ? (
                                <GripVertical size={20} />
                              ) : section === "rule_set" ? (
                                <Layers size={20} />
                              ) : (
                                <Radio size={20} />
                              )}
                            </div>
                            <button
                              className="node-info"
                              onClick={() => openEdit(i, v)}
                            >
                              <strong>
                                {tagNames(v).join(", ") || `Rule ${i + 1}`}
                              </strong>
                              {section === "rules" ? (
                                <span>
                                  WHEN{" "}
                                  {Object.entries(v)
                                    .filter(
                                      ([k]) =>
                                        !["action", "outbound"].includes(k),
                                    )
                                    .map(
                                      ([k, x]) =>
                                        `${k}: ${typeof x === "object" ? JSON.stringify(x) : x}`,
                                    )
                                    .join(" · ") || "all traffic"}{" "}
                                  <ArrowRight size={12} /> THEN{" "}
                                  {v.action || "route"} {v.outbound || ""}
                                </span>
                              ) : (
                                <span>
                                  {v.listen ||
                                    v.server ||
                                    v.url ||
                                    v.path ||
                                    "No address"}
                                  {v.listen_port || v.server_port
                                    ? `:${v.listen_port || v.server_port}`
                                    : ""}{" "}
                                  ·{" "}
                                  {v.tls?.enabled
                                    ? "TLS enabled"
                                    : "TLS not enabled"}
                                  {v.transport?.type
                                    ? ` · ${v.transport.type}`
                                    : ""}
                                  {v.method ? ` · ${v.method}` : ""}
                                </span>
                              )}
                            </button>
                            <span className="protocol">
                              {v.type || v.action || "route"}
                            </span>
                            <button
                              aria-label={`Duplicate ${v.tag || "rule"}`}
                              disabled={info.readOnly}
                              onClick={() =>
                                openEdit(-1, {
                                  ...structuredClone(v),
                                  ...(v.tag
                                    ? {
                                        tag: Array.isArray(v.tag)
                                          ? v.tag.map(
                                              (tag: string) => tag + "-copy",
                                            )
                                          : v.tag + "-copy",
                                      }
                                    : {}),
                                })
                              }
                            >
                              <Copy size={15} />
                            </button>
                            {section === "rules" && (
                              <>
                                <button
                                  aria-label="Move rule up"
                                  disabled={i === 0 || info.readOnly}
                                  onClick={() => {
                                    const rows = [...items];
                                    [rows[i - 1], rows[i]] = [
                                      rows[i],
                                      rows[i - 1],
                                    ];
                                    change(
                                      replaceCollection(config, section, rows),
                                    );
                                  }}
                                >
                                  ↑
                                </button>
                                <button
                                  aria-label="Move rule down"
                                  disabled={
                                    i === items.length - 1 || info.readOnly
                                  }
                                  onClick={() => {
                                    const rows = [...items];
                                    [rows[i + 1], rows[i]] = [
                                      rows[i],
                                      rows[i + 1],
                                    ];
                                    change(
                                      replaceCollection(config, section, rows),
                                    );
                                  }}
                                >
                                  ↓
                                </button>
                              </>
                            )}
                            <button
                              className="danger-button"
                              aria-label={`Delete ${v.tag || "rule"}`}
                              disabled={info.readOnly}
                              onClick={() => remove(i)}
                            >
                              <Trash2 size={15} />
                            </button>
                          </div>
                        ))}
                    </div>
                  ) : (
                    <div className="empty">
                      <Layers size={30} />
                      <h3>A clean starting point</h3>
                      <p>
                        Add your first{" "}
                        {section === "rules"
                          ? "routing rule"
                          : section === "rule_set"
                            ? "rule set"
                            : section.slice(0, -1)}{" "}
                        to your configuration draft.
                      </p>
                    </div>
                  )}
                  {!Object.keys(schema).length && (
                    <p className="warning">
                      The installed binary has no available schema. Use the JSON
                      editor and installed checker for version-safe editing.
                    </p>
                  )}
                  {section === "rules" && (
                    <p className="muted">
                      Rules run in order. Drag to reorder, or use the arrow
                      buttons. Conditions follow native sing-box matching
                      semantics; use a logical group for explicit AND / OR.
                    </p>
                  )}
                </section>
              )}
              {page === "JSON Editor" && (
                <section className="glass panel editor-panel">
                  <div className="toolbar">
                    <FileJson size={18} />
                    <strong>Native configuration</strong>
                    <span className="spacer" />
                    <button
                      onClick={() => {
                        try {
                          const v = JSON.parse(json);
                          if (!v || Array.isArray(v) || typeof v !== "object")
                            throw Error("Configuration must be an object");
                          change(v);
                        } catch (e) {
                          handleError(e);
                        }
                      }}
                    >
                      Format & synchronize
                    </button>
                    <button onClick={() => setDiff(true)}>View diff</button>
                  </div>
                  <Suspense
                    fallback={
                      <div className="empty glass" role="status">
                        Loading configuration editor…
                      </div>
                    }
                  >
                    <Editor
                      height="58vh"
                      language="json"
                      theme={light ? "light" : "vs-dark"}
                      value={json}
                      onChange={(v) => {
                        setJson(v || "");
                        try {
                          const c = JSON.parse(v || "");
                          if (c && typeof c === "object" && !Array.isArray(c))
                            setConfig(c);
                        } catch {
                          /* Keep invalid text without corrupting model */
                        }
                      }}
                      onMount={(_editor, m) => {
                        m.json.jsonDefaults.setDiagnosticsOptions({
                          validate: true,
                          schemas: Object.keys(schema).length
                            ? [
                                {
                                  uri: "https://sing-box.sagernet.org/schema.json",
                                  fileMatch: ["*"],
                                  schema,
                                },
                              ]
                            : [],
                        });
                      }}
                      options={{
                        readOnly: info.readOnly,
                        minimap: { enabled: false },
                        fontSize: 13,
                        padding: { top: 18 },
                        scrollBeyondLastLine: false,
                        automaticLayout: true,
                      }}
                    />
                  </Suspense>
                  <p className="muted">
                    Unknown fields are preserved. Invalid JSON must be corrected
                    before saving. Use Ctrl/Cmd+F to search and Ctrl/Cmd+H to
                    replace.
                  </p>
                </section>
              )}
              {page === "Settings" && (
                <div className="settings-grid">
                  <section className="glass panel">
                    <div className="section-title">
                      <h3>Installation</h3>
                      <Box size={18} />
                    </div>
                    {[
                      ["Version", status.version],
                      ["Service unit", status.unit],
                      ["Access mode", info.readOnly ? "Read-only" : "Managed"],
                      [
                        "Source files",
                        (info.sources || [])
                          .map((s: any) => (typeof s === "string" ? s : s.path))
                          .join(", "),
                      ],
                      [
                        "Monitoring",
                        telemetry.available
                          ? "Connected"
                          : telemetry.error || "Unavailable",
                      ],
                    ].map(([k, v]) => (
                      <div className="health-row" key={k}>
                        <span>{k}</span>
                        <strong>{v || "Unavailable"}</strong>
                      </div>
                    ))}
                    <p className="muted">
                      Tailscale HTTPS is configured by the installer. The
                      Manager listener remains on localhost; without Tailscale,
                      use an SSH tunnel.
                    </p>
                    <button
                      onClick={() =>
                        run(async () => {
                          await api("logout", "POST", {});
                          setAuth(true);
                        })
                      }
                    >
                      <LogOut size={16} /> Sign out
                    </button>
                  </section>
                  <section className="glass panel">
                    <div className="section-title">
                      <h3>Configuration history</h3>
                      <Layers size={18} />
                    </div>
                    <p className="muted">
                      Restore a previous version into a draft, then validate and
                      apply it.
                    </p>
                    {history.length ? (
                      history.map((h) => (
                        <div className="health-row" key={h.id}>
                          <span>
                            {h.created
                              ? new Date(h.created).toLocaleString()
                              : h.id}
                          </span>
                          <button
                            disabled={info.readOnly || busy}
                            onClick={() =>
                              run(async () => {
                                await api("restore", "POST", { id: h.id });
                                await load();
                                setMessage(
                                  "Historical configuration restored into a draft.",
                                );
                              })
                            }
                          >
                            Restore draft
                          </button>
                        </div>
                      ))
                    ) : (
                      <div className="empty-inline">
                        No configuration backups yet.
                      </div>
                    )}
                  </section>
                </div>
              )}
            </>
          )}
          {dirty && (
            <div className="draft-bar glass">
              <div>
                <span className="draft-dot" />
                <strong>Unapplied changes</strong>
                <small>Saved drafts never restart sing-box.</small>
              </div>
              <div>
                <button
                  disabled={busy || info.readOnly || conflict}
                  onClick={() =>
                    run(async () => {
                      if (pretty(JSON.parse(json)) !== pretty(config))
                        throw Error("Synchronize JSON first");
                      await save();
                    })
                  }
                >
                  Save draft
                </button>
                <button
                  disabled={busy || info.readOnly || conflict}
                  onClick={() => run(validate)}
                >
                  Validate
                </button>
                <button
                  className="primary"
                  disabled={busy || info.readOnly || conflict}
                  onClick={() => setDiff(true)}
                >
                  Review & apply <ArrowRight size={15} />
                </button>
              </div>
            </div>
          )}
          <footer>
            <span>
              <Box size={13} /> sing-box Manager
            </span>
            <span>Private infrastructure. Clearly managed.</span>
          </footer>
        </main>
      </div>
      {logs && (
        <Modal title="Live journal" onClose={() => setLogs(false)} wide>
          <Logs />
        </Modal>
      )}
      {diff && (
        <Modal
          title="Review configuration changes"
          onClose={() => setDiff(false)}
          wide
        >
          <p className="muted">
            Live configuration on the left, your draft on the right. Applying
            restarts sing-box; failed health checks trigger rollback.
          </p>
          {status.local && (
            <p className="notice warning">
              Local inspection mode supports drafts and validation. Applying
              requires the Linux deployment helper and a managed systemd
              service.
            </p>
          )}
          {error && (
            <p className="notice error" role="alert">
              {error}
            </p>
          )}
          <Suspense
            fallback={
              <div className="empty glass" role="status">
                Loading configuration editor…
              </div>
            }
          >
            <DiffEditor
              height="55vh"
              language="json"
              original={pretty(live)}
              modified={pretty(config)}
              theme={light ? "light" : "vs-dark"}
              options={{
                readOnly: true,
                minimap: { enabled: false },
                renderSideBySide: window.innerWidth > 700,
                automaticLayout: true,
              }}
            />
          </Suspense>
          <div className="modal-footer">
            <button onClick={() => setDiff(false)}>Keep editing</button>
            <button
              className="primary"
              disabled={
                busy || info.readOnly || status.local || conflict || !dirty
              }
              onClick={() =>
                run(async () => {
                  await validate();
                  await api("apply", "POST", {});
                  await load();
                  setDiff(false);
                  setMessage(
                    "Configuration applied and service health verified.",
                  );
                })
              }
            >
              Validate, back up & apply
            </button>
          </div>
        </Modal>
      )}
      {editing && (
        <Modal
          title={`${editing.index < 0 ? "Add" : "Edit"} ${editing.section === "rules" ? "routing rule" : editing.section === "rule_set" ? "rule set" : editing.section.slice(0, -1)}`}
          onClose={() => setEditing(null)}
        >
          <form
            onSubmit={(e) => {
              e.preventDefault();
              try {
                const v = rawMode ? JSON.parse(raw) : editing.value;
                if (!v || typeof v !== "object" || Array.isArray(v))
                  throw Error("Entry must be a JSON object");
                const rows = collection(config, editing.section);
                if (
                  v.tag &&
                  rows.some(
                    (x, i) =>
                      i !== editing.index &&
                      tagNames(x).some((tag) => tagNames(v).includes(tag)),
                  )
                )
                  throw Error("Tags must be unique");
                const next =
                  editing.index < 0
                    ? [...rows, v]
                    : rows.map((x, i) => (i === editing.index ? v : x));
                change(replaceCollection(config, editing.section, next));
                setEditing(null);
              } catch (e) {
                handleError(e);
              }
            }}
          >
            <div className="toolbar">
              <span className="pill">DRAFT ONLY</span>
              <div className="spacer" />
              <button
                type="button"
                onClick={() => {
                  if (rawMode) {
                    try {
                      setEditing({ ...editing, value: JSON.parse(raw) });
                      setRawMode(false);
                    } catch {
                      setError("Fix JSON before switching to visual mode");
                    }
                  } else {
                    setRaw(pretty(editing.value));
                    setRawMode(true);
                  }
                }}
              >
                {rawMode ? "Visual fields" : "JSON preview"}
              </button>
            </div>
            {error && (
              <p className="notice error" role="alert">
                {error}
              </p>
            )}
            <div className="editor-scroll">
              {rawMode ? (
                <textarea
                  className="raw-editor"
                  aria-label="Entry JSON"
                  value={raw}
                  onChange={(e) => setRaw(e.target.value)}
                  spellCheck={false}
                />
              ) : (
                <>
                  <label className="field">
                    <span>
                      {editing.section === "rules"
                        ? "Rule structure"
                        : "Protocol / type"}
                    </span>
                    <select
                      aria-label={
                        editing.section === "rules"
                          ? "Rule structure"
                          : "Protocol / type"
                      }
                      value={editing.value.type || ""}
                      onChange={(e) => {
                        const v = { ...editing.value };
                        if (e.target.value) v.type = e.target.value;
                        else delete v.type;
                        setEditing({ ...editing, value: v });
                      }}
                    >
                      {editing.section === "rules" && (
                        <option value="">Standard match</option>
                      )}
                      {types(schema, editing.section).map((t) => (
                        <option key={t}>{t}</option>
                      ))}
                    </select>
                  </label>
                  <SchemaForm
                    key={editing.value.type || "standard"}
                    root={schema}
                    schema={
                      editing.section === "rules"
                        ? matchSchema(schema, editing.value.type || "")
                        : objectSchema(
                            schema,
                            editing.section,
                            editing.value.type || "",
                          )
                    }
                    value={editing.value}
                    config={config}
                    onChange={(v) => setEditing({ ...editing, value: v })}
                  />
                  {editing.section === "rules" && (
                    <RuleAction
                      root={schema}
                      value={editing.value}
                      config={config}
                      onChange={(v) => setEditing({ ...editing, value: v })}
                    />
                  )}
                  <details className="json-preview">
                    <summary>Complete JSON preview</summary>
                    <pre>{pretty(editing.value)}</pre>
                  </details>
                  <p className="muted">
                    Fields come from your installed binary. Existing fields are
                    preserved when switching types; remove incompatible fields
                    in JSON before validation.
                  </p>
                </>
              )}
            </div>
            <div className="modal-footer">
              <button type="button" onClick={() => setEditing(null)}>
                Cancel
              </button>
              <button className="primary" disabled={info.readOnly}>
                Save to draft <Check size={15} />
              </button>
            </div>
          </form>
        </Modal>
      )}
    </div>
  );
}
function RuleAction({
  root,
  value,
  onChange,
  config,
}: {
  root: Obj;
  value: Obj;
  onChange: (v: Obj) => void;
  config: Obj;
}) {
  const actions = variants(root, root.$defs?.RuleAction || {});
  const choices = actions.flatMap((s) => {
    const p = resolve(root, s.properties?.action);
    return p.const ? [p.const] : p.enum || [];
  });
  const selected =
    actions.find(
      (s) => s.properties?.action?.const === (value.action || "route"),
    ) ||
    actions[0] ||
    {};
  return (
    <div className="rule-action">
      <h3>THEN</h3>
      <label className="field">
        <span>Action</span>
        <select
          aria-label="Action"
          value={value.action || "route"}
          onChange={(e) => onChange({ ...value, action: e.target.value })}
        >
          {choices.map((v: string) => (
            <option key={v}>{v}</option>
          ))}
        </select>
      </label>
      <SchemaForm
        root={root}
        schema={{
          ...selected,
          properties: Object.fromEntries(
            Object.entries(selected.properties || {}).filter(
              ([k]) => k !== "action",
            ),
          ),
        }}
        value={value}
        config={config}
        onChange={onChange}
      />
    </div>
  );
}
