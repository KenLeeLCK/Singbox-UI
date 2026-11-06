import { useEffect, useRef } from "react";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";
export function Chart({
  samples,
  color,
  field,
  minutes,
}: {
  samples: any[];
  color: string;
  field: string;
  minutes: number;
}) {
  const el = useRef<HTMLDivElement>(null);
  const plot = useRef<uPlot | null>(null);
  useEffect(() => {
    if (!el.current) return;
    const p = new uPlot(
      {
        width: el.current.clientWidth || 400,
        height: 130,
        legend: { show: false },
        cursor: { show: false },
        axes: [{ show: false }, { show: false }],
        scales: {
          x: { time: true },
          y: {
            range: (_s, _min, max) => [
              0,
              Number.isFinite(max) ? Math.max(max * 1.2, 1) : 1,
            ],
          },
        },
        series: [{}, { stroke: color, width: 2, fill: color + "18" }],
      },
      [[], []],
      el.current,
    );
    plot.current = p;
    const o = new ResizeObserver(() =>
      p.setSize({ width: el.current!.clientWidth, height: 130 }),
    );
    o.observe(el.current);
    return () => {
      o.disconnect();
      p.destroy();
    };
  }, [color]);
  useEffect(() => {
    const now = Date.now() / 1000;
    const rows = samples.filter(
      (s) => s.time >= now - minutes * 60 && s.time <= now,
    );
    plot.current?.setData([
      rows.map((s) => s.time),
      rows.map((s) => s[field] ?? null),
    ]);
    plot.current?.setScale("x", { min: now - minutes * 60, max: now });
  }, [samples, field, minutes]);
  return <div className="chart" ref={el} />;
}
