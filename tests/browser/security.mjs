// Isolated authentication regression. Never uses production tokens or services.
import { chromium } from "playwright";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import http from "node:http";
import { spawn } from "node:child_process";
import { httpsProxy } from "./https-proxy.mjs";
const root = path.resolve(import.meta.dirname, "../..");
const dir = await fs.mkdtemp(path.join(os.tmpdir(), "manager-security-"));
const reserve = http.createServer();
await new Promise((r) => reserve.listen(0, "127.0.0.1", r));
const port = reserve.address().port;
await new Promise((r) => reserve.close(r));
let manager, browser, tls;
try {
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
      "--login-attempts",
      "3",
      "--login-window",
      "2s",
      "--session-ttl",
      "3s",
    ],
    { stdio: "ignore" },
  );
  const base = `http://127.0.0.1:${port}`;
  for (let n = 0; n < 100; n++) {
    try {
      if ((await fetch(base + "/healthz")).ok) break;
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  const token = (
    await fs.readFile(path.join(dir, "state/token"), "utf8")
  ).trim();
  assert.equal(token.length, 64);
  assert.equal(
    (await fs.stat(path.join(dir, "state/token"))).mode & 0o777,
    0o600,
  );
  tls = await httpsProxy(base, dir);
  browser = await chromium.launch();
  for (const url of [base, tls.url]) {
    const context = await browser.newContext({ ignoreHTTPSErrors: true });
    const page = await context.newPage();
    const errors = [];
    page.on("pageerror", (e) => errors.push(e.message));
    const response = await page.goto(url);
    assert.equal(response.status(), 200);
    assert.equal(response.headers()["www-authenticate"], undefined);
    assert.equal(response.headers()["x-frame-options"], "DENY");
    assert.equal(response.headers()["referrer-policy"], "no-referrer");
    assert.ok(
      response.headers()["content-security-policy"].includes("base-uri 'none'"),
    );
    await page.getByLabel("Manager access token").waitFor();
    const post = async (token, extra = {}) =>
      context.request.post(url + "/api/login", {
        data: { token },
        headers: {
          "X-Requested-With": "sing-box-manager",
          Origin: url,
          ...extra,
        },
      });
    for (const route of [
      "config",
      "status",
      "schema",
      "history",
      "logs",
      "telemetry",
    ])
      assert.equal(
        (await context.request.get(url + "/api/" + route)).status(),
        401,
      );
    for (const route of ["validate", "apply", "restore", "monitoring/prepare"])
      assert.equal(
        (
          await context.request.post(url + "/api/" + route, { data: {} })
        ).status(),
        401,
      );
    assert.equal(
      (await context.request.put(url + "/api/draft", { data: {} })).status(),
      401,
    );
    assert.equal(
      (await post(token, { Origin: "https://evil.test" })).status(),
      403,
    );
    assert.equal(
      (
        await context.request.get(url + "/api/config", {
          headers: { Cookie: "manager_session=forged" },
        })
      ).status(),
      401,
    );
    for (let n = 0; n < 3; n++)
      assert.equal(
        (
          await post("incorrect", {
            "X-Forwarded-For": `198.51.100.${n}`,
            "X-Real-IP": `203.0.113.${n}`,
          })
        ).status(),
        401,
      );
    const limited = await post(token, { "X-Forwarded-For": "8.8.8.8" });
    assert.equal(limited.status(), 429);
    assert.ok(Number(limited.headers()["retry-after"]) > 0);
    await page.waitForTimeout(
      Number(limited.headers()["retry-after"]) * 1000 + 100,
    );
    await page.getByLabel("Manager access token").fill(token);
    await page.getByRole("button", { name: "Unlock Manager" }).click();
    await page
      .getByRole("heading", { name: "A clearer view.", exact: true })
      .waitFor();
    const cookie = (await context.cookies()).find(
      (c) => c.name === "manager_session",
    );
    assert.ok(cookie.httpOnly && cookie.sameSite === "Strict");
    assert.equal(cookie.secure, url.startsWith("https:"));
    await page.reload();
    await page
      .getByRole("heading", { name: "A clearer view.", exact: true })
      .waitFor();
    assert.equal(
      (
        await context.request.post(url + "/api/validate", {
          headers: { Origin: "https://evil.test" },
          data: {},
        })
      ).status(),
      403,
    );
    // Real server expiry, not a mocked 401. The UI's regular poll returns to login.
    await page.getByLabel("Manager access token").waitFor({ timeout: 15000 });
    assert.equal(
      (
        await context.request.get(url + "/api/config", {
          headers: { Cookie: `manager_session=${cookie.value}` },
        })
      ).status(),
      401,
    );
    await page.getByLabel("Manager access token").fill(token);
    await page.getByRole("button", { name: "Unlock Manager" }).click();
    await page
      .getByRole("heading", { name: "A clearer view.", exact: true })
      .waitFor();
    const current = (await context.cookies()).find(
      (c) => c.name === "manager_session",
    );
    assert.notEqual(current.value, cookie.value);
    await page.evaluate(() =>
      fetch("/api/logout", {
        method: "POST",
        headers: { "X-Requested-With": "sing-box-manager" },
      }),
    );
    assert.equal(
      (
        await context.request.get(url + "/api/config", {
          headers: { Cookie: `manager_session=${current.value}` },
        })
      ).status(),
      401,
    );
    assert.deepEqual(errors, []);
    await context.close();
    console.log(
      "PASS token-only security:",
      new URL(url).protocol,
      "API denial, CSRF, throttle/spoof resistance/recovery, secure cookies, real expiry, logout, headers",
    );
  }
} finally {
  await browser?.close();
  await tls?.close();
  manager?.kill("SIGTERM");
  await fs.rm(dir, { recursive: true, force: true });
}
