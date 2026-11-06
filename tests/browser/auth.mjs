// Isolated authentication and failure recovery; never uses production credentials.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import http from "node:http";
import https from "node:https";
import { spawn, execFileSync } from "node:child_process";
import { diagnostics } from "./diagnostics.mjs";
const root = path.resolve(import.meta.dirname, "../..");
const dir = await fs.mkdtemp(path.join(os.tmpdir(), "manager-auth-"));
const artifacts = path.join(
  root,
  ".local/browser-artifacts/auth-" + Date.now(),
);
const diagnostic = await diagnostics(artifacts);
let manager, browser, proxy, context, page;
const errors = [];
const secrets = new Set();
let failed = false;
const listen = (s) =>
  new Promise((r) => s.listen(0, "127.0.0.1", () => r(s.address().port)));
try {
  const reserve = http.createServer();
  const port = await listen(reserve);
  await new Promise((r) => reserve.close(r));
  await fs.writeFile(
    path.join(dir, "config.json"),
    '{"outbounds":[{"type":"direct","tag":"direct"}]}',
  );
  manager = spawn(
    process.env.MANAGER_BIN || path.join(root, "artifacts/manager"),
    [
      "serve",
      "--local",
      "--state-dir",
      path.join(dir, "state"),
      "--config",
      path.join(dir, "config.json"),
      "--sing-box",
      process.env.SING_BOX_BIN,
      "--port",
      String(port),
      "--port-max",
      String(port),
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  diagnostic.process("manager", manager);
  const base = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 100; i++) {
    try {
      if ((await fetch(base + "/healthz")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  const token = (
    await fs.readFile(path.join(dir, "state/token"), "utf8")
  ).trim();
  secrets.add(token);
  secrets.add("invalid-test-token");
  execFileSync(
    "openssl",
    [
      "req",
      "-x509",
      "-newkey",
      "rsa:2048",
      "-nodes",
      "-keyout",
      path.join(dir, "key.pem"),
      "-out",
      path.join(dir, "cert.pem"),
      "-subj",
      "/CN=localhost",
      "-days",
      "1",
    ],
    { stdio: "ignore" },
  );
  const seen = [];
  proxy = https.createServer(
    {
      key: await fs.readFile(path.join(dir, "key.pem")),
      cert: await fs.readFile(path.join(dir, "cert.pem")),
    },
    (req, res) => {
      seen.push({
        path: req.url,
        host: req.headers.host,
        origin: req.headers.origin,
        cookie: !!req.headers.cookie,
      });
      const upstream = http.request(
        base + req.url,
        {
          method: req.method,
          headers: {
            ...req.headers,
            "x-forwarded-proto": "https",
            "x-forwarded-host": req.headers.host,
          },
        },
        (r) => {
          res.writeHead(r.statusCode, r.headers);
          r.pipe(res);
        },
      );
      upstream.on("error", () => {
        res.writeHead(502);
        res.end();
      });
      res.on("close", () => upstream.destroy());
      req.pipe(upstream);
    },
  );
  const tlsPort = await listen(proxy);
  browser = await chromium.launch();
  for (const url of [base, `https://localhost:${tlsPort}`]) {
    context = await browser.newContext({ ignoreHTTPSErrors: true });
    await context.tracing.start({ screenshots: false, snapshots: true });
    page = await context.newPage();
    page.on("response", (r) => {
      const c = r
        .headers()
        ["set-cookie"]?.match(/manager_session=([^;]+)/)?.[1];
      if (c) secrets.add(c);
    });
    page.setDefaultTimeout(10000);
    diagnostic.page(page);
    page.on("pageerror", (e) => errors.push(e.message));
    const editorRequests = [];
    page.on("request", (r) => {
      if (/CodeEditor|editor\.api/.test(r.url())) editorRequests.push(r.url());
    });
    const login = async () => {
      await page.getByLabel("Manager access token").fill(token);
      await page.getByRole("button", { name: "Unlock Manager" }).click();
    };
    const dashboard = () =>
      page
        .getByRole("heading", { name: "A clearer view.", exact: true })
        .waitFor();
    await page.goto(url);
    await page.getByLabel("Manager access token").fill("invalid-test-token");
    await page.getByRole("button", { name: "Unlock Manager" }).click();
    await page.getByText("Invalid access token.", { exact: true }).waitFor();
    await page.route("**/api/config", (r) =>
      r.fulfill({ status: 503, body: "unavailable" }),
    );
    await login();
    await page
      .getByRole("heading", { name: "Unable to load configuration" })
      .waitFor();
    await page.unroute("**/api/config");
    await page.getByRole("button", { name: "Retry loading" }).click();
    await dashboard();
    assert.equal(
      editorRequests.length,
      0,
      "login/dashboard must not fetch Monaco",
    );
    const cookie = (await context.cookies()).find(
      (c) => c.name === "manager_session",
    );
    assert.ok(cookie && cookie.httpOnly && cookie.sameSite === "Strict");
    assert.equal(cookie.secure, url.startsWith("https:"));
    for (const endpoint of ["config", "status", "schema", "history"])
      assert.equal(
        await page.evaluate(
          async (endpoint) => (await fetch("/api/" + endpoint)).status,
          endpoint,
        ),
        200,
      );
    let releaseSchema;
    const schemaGate = new Promise((resolve) => {
      releaseSchema = resolve;
    });
    await page.route("**/api/schema", async (route) => {
      await schemaGate;
      await route.continue();
    });
    await page.reload();
    await dashboard(); // Must render even while schema has not responded.
    releaseSchema();
    await page.unroute("**/api/schema");
    await page.reload();
    await dashboard();
    await page.route("**/api/status", (r) =>
      r.fulfill({ status: 503, body: "unavailable" }),
    );
    await page.reload();
    await dashboard();
    await page
      .getByRole("alert")
      .filter({ hasText: "Some installation details" })
      .waitFor();
    await page.unroute("**/api/status");
    await page.route("**/api/config", (r) =>
      r.fulfill({ status: 503, body: "unavailable" }),
    );
    await page.reload();
    await page
      .getByRole("heading", { name: "Unable to load configuration" })
      .waitFor();
    await page.unroute("**/api/config");
    await page.getByRole("button", { name: "Retry loading" }).click();
    await dashboard();
    await page.route("**/api/telemetry", (r) => r.abort());
    await page.reload();
    await dashboard();
    assert.equal(await page.getByLabel("Manager access token").count(), 0);
    await page.unroute("**/api/telemetry");
    await page.route("**/api/config", (r) =>
      r.fulfill({ status: 401, body: "session expired" }),
    );
    await page.getByLabel("Manager access token").waitFor();
    await page.unroute("**/api/config");
    await login();
    await dashboard();
    const logoutCookie = (await context.cookies()).find(
      (c) => c.name === "manager_session",
    );
    const nav = (name) => page.locator("nav button").filter({ hasText: name });
    await nav("Settings").click();
    await page.getByRole("button", { name: /Sign out/ }).click();
    await page.getByLabel("Manager access token").waitFor();
    assert.equal(
      (await context.cookies()).filter((c) => c.name === "manager_session")
        .length,
      0,
    );
    assert.equal(
      (
        await context.request.get(url + "/api/config", {
          headers: { Cookie: `manager_session=${logoutCookie.value}` },
        })
      ).status(),
      401,
    );
    await login();
    await dashboard();
    await page.getByRole("button", { name: "Live Logs" }).click();
    await page.getByLabel("Search logs").waitFor();
    await page.getByRole("button", { name: "Close", exact: true }).click();
    await nav("JSON Editor").click();
    await page.locator(".monaco-editor textarea").first().focus();
    await page.keyboard.press(
      process.platform === "darwin" ? "Meta+A" : "Control+A",
    );
    await page.keyboard.press("Backspace");
    await page.keyboard.insertText(
      JSON.stringify({
        outbounds: [{ type: "direct", tag: "imported-direct" }],
      }),
    );
    await page.getByRole("button", { name: "Format & synchronize" }).click();
    await nav("Outbounds").click();
    await page.getByText("imported-direct", { exact: true }).waitFor();
    await page.getByRole("button", { name: "Save draft", exact: true }).click();
    assert.equal(
      await page.evaluate(
        async () =>
          (await (await fetch("/api/config")).json()).draft.config.outbounds[0]
            .tag,
      ),
      "imported-direct",
    );
    await nav("Overview").click();
    console.log("PASS JSON configuration import and draft persistence");
    await page.screenshot({
      path: path.join(
        artifacts,
        url.startsWith("https:") ? "https.png" : "http.png",
      ),
    });
    await context.tracing.stop();
    await context.close();
    context = null;
    console.log(
      "PASS authentication, cookies, reload, expiry, logout, retries, optional status and SSE:",
      new URL(url).protocol,
    );
  }
  assert.ok(
    seen.some(
      (r) =>
        r.path === "/api/login" &&
        r.origin === `https://localhost:${tlsPort}` &&
        r.host === `localhost:${tlsPort}`,
    ),
  );
  assert.ok(seen.some((r) => r.path === "/api/config" && r.cookie));
  assert.deepEqual(errors, []);
} catch (e) {
  failed = true;
  await diagnostic.failure(page, e);
  throw e;
} finally {
  if (failed && context) {
    const trace = path.join(artifacts, "trace.zip");
    try {
      await context.tracing.stop({ path: trace });
      execFileSync(
        "python3",
        [path.join(root, "tests/browser/sanitize-trace.py"), trace],
        {
          input: JSON.stringify([...secrets]),
          stdio: ["pipe", "ignore", "pipe"],
        },
      );
    } catch {
      await fs.rm(trace, { force: true });
    }
  }
  await context?.close();
  await browser?.close();
  proxy?.closeAllConnections();
  proxy?.close();
  manager?.kill("SIGTERM");
  await diagnostic.save();
  await fs.rm(dir, { recursive: true, force: true });
}
