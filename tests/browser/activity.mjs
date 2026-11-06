// Production frontend with synthetic telemetry, isolated from all live services.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import http from "node:http";
import fs from "node:fs/promises";
import path from "node:path";
const dist = path.resolve(
  import.meta.dirname,
  "../../backend/internal/web/dist",
);
const artifacts = path.resolve(
  import.meta.dirname,
  "../../.local/browser-artifacts/activity",
);
await fs.mkdir(artifacts, { recursive: true });
const streams = new Set();
const connection = (n) => ({
  id: `connection-${n}`,
  createdAt: n,
  destination: `host-${n}.test`,
  inbound: "in",
  outbound: "direct",
  network: "tcp",
});
let entries = [];
const sample = () => ({
  available: true,
  connectionsAvailable: true,
  connections: entries,
  activeConnections: entries.length,
  uploadRate: 123,
  downloadRate: 456,
  uploadTotal: 1000,
  downloadTotal: 2000,
});
const send = () => {
  for (const res of streams)
    res.write(`event: telemetry\ndata: ${JSON.stringify(sample())}\n\n`);
};
const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, "http://localhost");
  if (url.pathname === "/api/telemetry") {
    res.writeHead(200, { "Content-Type": "text/event-stream" });
    streams.add(res);
    send();
    req.on("close", () => streams.delete(res));
    return;
  }
  if (url.pathname.startsWith("/api/")) {
    res.setHeader("Content-Type", "application/json");
    res.end(
      JSON.stringify(
        url.pathname === "/api/config"
          ? {
              config: { inbounds: [], outbounds: [], route: {} },
              hash: "activity-fixture",
              readOnly: true,
            }
          : url.pathname === "/api/history"
            ? []
            : {},
      ),
    );
    return;
  }
  try {
    const file = path.resolve(
      dist,
      "." + (url.pathname === "/" ? "/index.html" : url.pathname),
    );
    if (!file.startsWith(dist + path.sep)) {
      res.writeHead(403).end();
      return;
    }
    res.setHeader(
      "Content-Type",
      file.endsWith(".js")
        ? "text/javascript"
        : file.endsWith(".css")
          ? "text/css"
          : "text/html",
    );
    res.end(await fs.readFile(file));
  } catch {
    res.writeHead(404).end();
  }
});
await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
const browser = await chromium.launch();
try {
  for (const width of [1280, 390]) {
    const context = await browser.newContext({
      viewport: { width, height: 900 },
      reducedMotion: width === 390 ? "reduce" : "no-preference",
    });
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    entries = Array.from({ length: 75 }, (_, n) => connection(n + 1));
    await page.goto(`http://127.0.0.1:${server.address().port}`);
    const rows = page.locator(".activity-list tbody tr");
    const count = async (n) =>
      page.waitForFunction(
        (n) =>
          document.querySelectorAll(".activity-list tbody tr").length === n,
        n,
      );
    const update = async (items) => {
      entries = items;
      send();
      await page.waitForFunction(
        (n) =>
          document.querySelector(".connections .pill")?.textContent.trim() ===
          `${n} CONNECTIONS`,
        items.length,
      );
    };
    await count(10);
    await page
      .locator(".connections")
      .screenshot({ path: path.join(artifacts, `collapsed-${width}.png`) });
    assert.equal(
      await rows.first().locator("td").first().textContent(),
      "host-75.test",
    );
    await page
      .getByRole("button", { name: "Show All (65)", exact: true })
      .focus();
    await page.keyboard.press("Enter");
    await count(75);
    assert.equal(
      await page
        .getByRole("button", { name: "Collapse", exact: true })
        .getAttribute("aria-expanded"),
      "true",
    );
    await page
      .locator(".connections")
      .screenshot({ path: path.join(artifacts, `expanded-${width}.png`) });
    const viewport = page.locator(".activity-list");
    assert.ok(
      await viewport.evaluate(
        (el) => el.clientHeight <= 480 && el.scrollHeight > el.clientHeight,
      ),
    );
    await viewport.evaluate((el) => {
      el.scrollTop = 650;
    });
    const anchor = await viewport.evaluate((el) => {
      const top = el.getBoundingClientRect().top;
      const rows = [...el.querySelectorAll("tbody tr")].filter(
        (r) => r.getBoundingClientRect().bottom > top,
      );
      return rows.slice(0, 2).map((r) => ({
        id: r.dataset.connectionId,
        offset: r.getBoundingClientRect().top - top,
      }));
    });
    // Remove the visible row while prepending hundreds of new connections.
    await update(
      Array.from({ length: 500 }, (_, n) => connection(n + 1)).filter(
        (c) => c.id !== anchor[0].id,
      ),
    );
    await count(499);
    const offset = await viewport.evaluate(
      (el, id) =>
        [...el.querySelectorAll("tbody tr")]
          .find((r) => r.dataset.connectionId === id)
          .getBoundingClientRect().top - el.getBoundingClientRect().top,
      anchor[1].id,
    );
    assert.ok(
      Math.abs(offset - anchor[1].offset) < 2,
      `scroll anchor shifted at ${width}px`,
    );
    await page.evaluate(() => {
      window.rowMutations = 0;
      window.rowObserver = new MutationObserver(
        (ms) => (window.rowMutations += ms.length),
      );
      window.rowObserver.observe(
        document.querySelector(".activity-list tbody"),
        {
          subtree: true,
          childList: true,
          attributes: true,
          characterData: true,
        },
      );
    });
    entries = entries.map((c) => ({ ...c, uploadTotal: 999 }));
    send();
    await page.waitForTimeout(150);
    assert.equal(
      await page.evaluate(() => window.rowMutations),
      0,
      "counter-only samples must not mutate rows",
    );
    await page.evaluate(() => window.rowObserver.disconnect());
    await page.getByRole("button", { name: "Collapse", exact: true }).click();
    await count(10);
    assert.equal(await viewport.evaluate((el) => el.scrollTop), 0);
    await page
      .getByRole("button", { name: "Show All (489)", exact: true })
      .click();
    await count(499);
    // Empty/reconnected feeds preserve the user's expanded preference.
    await update([]);
    await count(0);
    await page.getByText("No active connections", { exact: true }).waitFor();
    await update(Array.from({ length: 11 }, (_, n) => connection(n)));
    await count(11);
    await page.getByRole("button", { name: "Collapse", exact: true }).click();
    await count(10);
    for (const n of [10, 1, 0]) {
      await update(Array.from({ length: n }, (_, i) => connection(i)));
      await count(n);
      assert.equal(await page.locator(".activity-toggle").count(), 0);
    }
    await update(Array.from({ length: 11 }, (_, i) => connection(i)));
    await count(10);
    await page
      .getByRole("button", { name: "Show All (1)", exact: true })
      .waitFor();
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    assert.deepEqual(errors, []);
    console.log(
      `PASS activity at ${width}px: newest-first, 0/1/10/11/75/499 rows, keyboard expand/collapse, bounded scrolling, live anchoring, stable empty-feed state, no counter-only row mutations`,
    );
    await context.close();
  }
} finally {
  await browser.close();
  for (const res of streams) res.end();
  await new Promise((resolve) => server.close(resolve));
}
