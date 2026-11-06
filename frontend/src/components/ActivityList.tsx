import { createRef, memo, PureComponent } from "react";
import { ChevronDown } from "lucide-react";
import type { Obj } from "../lib/model";

const PREVIEW_COUNT = 10;
const ActivityRow = memo(function ActivityRow({
  id,
  destination,
  inbound,
  outbound,
  network,
}: {
  id: string;
  destination: string;
  inbound: string;
  outbound: string;
  network: string;
}) {
  return (
    <tr data-connection-id={id}>
      <td>{destination}</td>
      <td>{inbound}</td>
      <td>{outbound}</td>
      <td>{network}</td>
    </tr>
  );
});

type Props = { connections: Obj[] };
type State = { expanded: boolean };
type Anchor = { id: string; offset: number } | null;

// Snapshot before DOM mutations so live insertions/removals preserve the row
// being read, including when the first visible connection closes.
export class ActivityList extends PureComponent<Props, State, Anchor> {
  state: State = { expanded: false };
  private viewport = createRef<HTMLDivElement>();
  private input?: Obj[];
  private ordered: Obj[] = [];

  getSnapshotBeforeUpdate(): Anchor {
    const viewport = this.viewport.current;
    if (!this.state.expanded || !viewport || viewport.scrollTop === 0)
      return null;
    const remaining = new Set(this.props.connections.map((c) => c.id));
    const top = viewport.getBoundingClientRect().top;
    for (const row of viewport.querySelectorAll<HTMLTableRowElement>(
      "tbody tr",
    )) {
      if (
        row.getBoundingClientRect().bottom > top &&
        remaining.has(row.dataset.connectionId)
      ) {
        return {
          id: row.dataset.connectionId!,
          offset: row.getBoundingClientRect().top - top,
        };
      }
    }
    return null;
  }

  componentDidUpdate(_props: Props, previous: State, anchor: Anchor) {
    const viewport = this.viewport.current;
    if (!viewport) return;
    if (previous.expanded !== this.state.expanded) {
      viewport.scrollTop = 0;
    } else if (anchor) {
      const row = Array.from(
        viewport.querySelectorAll<HTMLTableRowElement>("tbody tr"),
      ).find((r) => r.dataset.connectionId === anchor.id);
      if (row)
        viewport.scrollTop +=
          row.getBoundingClientRect().top -
          viewport.getBoundingClientRect().top -
          anchor.offset;
    }
  }

  render() {
    if (this.input !== this.props.connections) {
      this.input = this.props.connections;
      // Native creation times, not traffic counters or snapshot arrival order.
      this.ordered = [...this.input].sort(
        (a, b) =>
          (b.createdAt || 0) - (a.createdAt || 0) ||
          String(a.id).localeCompare(String(b.id)),
      );
    }
    if (!this.ordered.length) return null;
    const { expanded } = this.state;
    const hidden = Math.max(0, this.ordered.length - PREVIEW_COUNT);
    const visible = expanded
      ? this.ordered
      : this.ordered.slice(0, PREVIEW_COUNT);
    return (
      <>
        <div
          id="active-activity-list"
          ref={this.viewport}
          className={`table-wrap activity-list${expanded ? " is-expanded" : ""}`}
          tabIndex={expanded ? 0 : undefined}
          role={expanded ? "region" : undefined}
          aria-label={expanded ? "Active connections" : undefined}
        >
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
              {visible.map((c) => (
                <ActivityRow
                  key={c.id}
                  id={c.id}
                  destination={
                    c.destination || c.host || c.metadata?.host || "—"
                  }
                  inbound={c.inbound || c.metadata?.inboundName || "—"}
                  outbound={c.outbound || c.chains?.join(" → ") || "—"}
                  network={c.network || c.metadata?.network || "—"}
                />
              ))}
            </tbody>
          </table>
        </div>
        {(hidden > 0 || expanded) && (
          <div className="activity-controls">
            <button
              className="text-button activity-toggle"
              aria-expanded={expanded}
              aria-controls="active-activity-list"
              onClick={() => this.setState({ expanded: !expanded })}
            >
              {expanded ? "Collapse" : `Show All (${hidden})`}
              <ChevronDown
                size={15}
                className={expanded ? "is-expanded" : ""}
              />
            </button>
          </div>
        )}
      </>
    );
  }
}
