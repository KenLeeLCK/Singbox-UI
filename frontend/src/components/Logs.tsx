import { useEffect, useRef, useState } from "react";
import { Pause, Play, Download, Copy, Trash2, Maximize2 } from "lucide-react";
export function Logs() {
  const [lines, setLines] = useState<string[]>([]),
    [paused, setPaused] = useState(false),
    [query, setQuery] = useState(""),
    [level, setLevel] = useState("ALL"),
    [state, setState] = useState("Connecting…"),
    [full, setFull] = useState(false);
  const end = useRef<HTMLDivElement>(null),
    pause = useRef(paused);
  pause.current = paused;
  useEffect(() => {
    const es = new EventSource("/api/logs");
    es.onopen = () => setState("Streaming journal");
    es.onmessage = (e) => {
      try {
        const d = JSON.parse(e.data);
        if (!pause.current)
          setLines((v) => [...v, d.line ?? String(d)].slice(-2000));
      } catch {
        /* Ignore malformed frames */
      }
    };
    es.onerror = () => setState("Disconnected · reconnecting");
    es.addEventListener("error", (e) => {
      if ((e as MessageEvent).data) {
        try {
          setState(JSON.parse((e as MessageEvent).data).error);
        } catch {
          setState("Journal unavailable");
        }
      }
    });
    return () => es.close();
  }, []);
  useEffect(() => {
    if (!paused) end.current?.scrollIntoView({ block: "nearest" });
  }, [lines, paused]);
  const visible = lines.filter(
    (l) =>
      l.toLowerCase().includes(query.toLowerCase()) &&
      (level === "ALL" || l.toUpperCase().includes(level)),
  );
  return (
    <div className={full ? "logs expanded" : "logs"}>
      <div className="toolbar">
        <span className="status-dot" />
        {state}
        <div className="spacer" />
        <button onClick={() => setPaused(!paused)}>
          {paused ? <Play size={15} /> : <Pause size={15} />}{" "}
          {paused ? "Resume" : "Pause"}
        </button>
        <button aria-label="Maximize logs" onClick={() => setFull(!full)}>
          <Maximize2 size={16} />
        </button>
      </div>
      <div className="toolbar">
        <input
          aria-label="Search logs"
          placeholder="Search journal…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <select
          aria-label="Log level"
          value={level}
          onChange={(e) => setLevel(e.target.value)}
        >
          {["ALL", "INFO", "WARN", "ERROR"].map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
        <button
          aria-label="Copy selected or visible logs"
          onClick={() =>
            navigator.clipboard.writeText(
              window.getSelection()?.toString() || visible.join("\n"),
            )
          }
        >
          <Copy size={16} />
        </button>
        <button
          aria-label="Export logs"
          onClick={() => {
            const a = document.createElement("a");
            a.href = URL.createObjectURL(
              new Blob([visible.join("\n")], { type: "text/plain" }),
            );
            a.download = "sing-box-journal.txt";
            a.click();
            URL.revokeObjectURL(a.href);
          }}
        >
          <Download size={16} />
        </button>
        <button aria-label="Clear visible buffer" onClick={() => setLines([])}>
          <Trash2 size={16} />
        </button>
      </div>
      <div className="log-lines">
        {visible.length ? (
          visible.map((l, i) => (
            <div
              key={i}
              className={
                /error|fatal/i.test(l)
                  ? "error"
                  : /warn/i.test(l)
                    ? "warning"
                    : ""
              }
            >
              {l}
            </div>
          ))
        ) : (
          <p>
            No journal entries. Only the configured service unit is streamed.
          </p>
        )}
        <div ref={end} />
      </div>
      <small>
        2,000 entry buffer · identifiable secrets redacted by server · clearing
        does not modify system logs
      </small>
    </div>
  );
}
