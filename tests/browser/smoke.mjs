import { chromium } from "playwright";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import net from "node:net";
import { spawn } from "node:child_process";
import crypto from "node:crypto";
import { diagnostics } from "./diagnostics.mjs";
const root = path.resolve(import.meta.dirname, "../..");
const binary = process.env.MANAGER_BIN || path.join(root, "artifacts/manager");
const singbox = process.env.SING_BOX_BIN;
if (!singbox)
  throw Error(
    "Set SING_BOX_BIN to a verified local binary; this suite uses only isolated temporary files and loopback ports.",
  );
const dir = await fs.mkdtemp(path.join(os.tmpdir(), "manager-browser-"));
const artifactDir = path.resolve(
  process.env.BROWSER_ARTIFACT_DIR ||
    path.join(
      root,
      ".local/browser-artifacts",
      new Date().toISOString().replaceAll(":", "-"),
    ),
);
const diagnostic = await diagnostics(artifactDir);
const screenshotDir =
  process.env.UPDATE_SCREENSHOTS === "1"
    ? path.join(root, "docs/screenshots")
    : artifactDir;
let browser, context, manager, proxy, page;
let failed = false;
const errors = [];
const deferEditorIdle = process.argv.includes("--defer-editor-idle");
async function ports() {
  // Keep both reservations open until allocated: closing the first before
  // asking for the second lets Linux return the same ephemeral port twice.
  const servers = [net.createServer(), net.createServer()];
  try {
    for (const server of servers)
      await new Promise((resolve, reject) => {
        server.once("error", reject);
        server.listen(0, "127.0.0.1", resolve);
      });
    return servers.map((server) => server.address().port);
  } finally {
    await Promise.all(
      servers.map((server) => new Promise((resolve) => server.close(resolve))),
    );
  }
}
async function wait(fn) {
  for (let i = 0; i < 100; i++) {
    try {
      const v = await fn();
      if (v) return v;
    } catch {}
    await new Promise((r) => setTimeout(r, 100));
  }
  throw Error("Readiness timeout");
}
try {
  const [apiPort, webPort] = await ports();
  assert.notEqual(apiPort, webPort, "fixture services need distinct ports");
  const live = {
    log: { level: "error" },
    inbounds: [],
    outbounds: [{ type: "direct", tag: "direct" }],
    services: [
      {
        type: "api",
        tag: "manager-api",
        listen: "127.0.0.1",
        listen_port: apiPort,
        secret: crypto.randomBytes(24).toString("hex"),
      },
    ],
  };
  const configPath = path.join(dir, "config.json");
  await fs.writeFile(configPath, JSON.stringify(live), { mode: 0o600 });
  proxy = spawn(singbox, ["run", "-c", configPath], {
    stdio: ["ignore", "pipe", "pipe"],
  });
  manager = spawn(
    binary,
    [
      "serve",
      "--local",
      "--state-dir",
      path.join(dir, "state"),
      "--config",
      configPath,
      "--sing-box",
      singbox,
      "--port",
      String(webPort),
      "--port-max",
      String(webPort),
    ],
    { stdio: ["ignore", "pipe", "pipe"] },
  );
  diagnostic.process("sing-box", proxy);
  diagnostic.process("manager", manager);
  const base = `http://127.0.0.1:${webPort}`;
  await wait(async () => (await fetch(base + "/healthz")).ok);
  const token = (
    await fs.readFile(path.join(dir, "state/token"), "utf8")
  ).trim();
  browser = await chromium.launch();
  context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
  });
  await context.tracing.start({
    screenshots: true,
    snapshots: true,
    sources: true,
  });
  page = await context.newPage();
  diagnostic.page(page);
  page.setDefaultTimeout(10000);
  page.on("pageerror", (e) => {
    errors.push({ name: e.name, message: e.message, stack: e.stack });
    console.error("BROWSER ERROR STACK", e.stack);
  });
  if (deferEditorIdle) {
    // Model a busy renderer deterministically. Flush idle work only after the
    // diff's services have been disposed, preserving cancellation semantics.
    await page.addInitScript(() => {
      const callbacks = new Map();
      let next = 0;
      window.requestIdleCallback = (callback) => {
        const id = ++next;
        callbacks.set(id, callback);
        return id;
      };
      window.cancelIdleCallback = (id) => callbacks.delete(id);
      window.flushTestIdle = () => {
        while (callbacks.size) {
          const [id, callback] = callbacks.entries().next().value;
          callbacks.delete(id);
          callback({ didTimeout: false, timeRemaining: () => 50 });
        }
      };
    });
  }
  console.log(
    "Browser scheduling:",
    deferEditorIdle ? "deferred idle regression" : "native",
  );
  await page.goto(base);
  await page.getByLabel("Manager access token").fill(token);
  await page.getByRole("button", { name: "Unlock Manager" }).click();
  await page.getByRole("heading", { name: "A clearer view." }).waitFor();
  await page.getByText("Native API connected", { exact: true }).waitFor();
  console.log("PASS authenticated dashboard with real native telemetry");
  const nav = (name) => page.locator("nav button").filter({ hasText: name });
  const dialog = () => page.getByRole("dialog");
  await nav("Inbounds").click();
  await page.getByRole("button", { name: "Add inbound", exact: true }).click();
  await dialog().locator("select").first().selectOption("mixed");
  await dialog()
    .getByLabel(/^tag\s*\*?$/i)
    .fill("qa-inbound");
  await dialog().getByLabel("listen", { exact: true }).fill("127.0.0.1");
  await dialog().getByLabel("listen port", { exact: true }).fill("18081");
  await dialog().getByRole("button", { name: "Save to draft" }).click();
  await page.getByRole("button", { name: "Save draft", exact: true }).click();
  await page.getByRole("button", { name: "Validate", exact: true }).click();
  await page
    .getByRole("status")
    .filter({ hasText: "validation passed" })
    .waitFor();
  console.log(
    "PASS schema-driven inbound create, draft persistence and installed checker",
  );
  await page
    .getByRole("button", { name: /qa-inbound/ })
    .first()
    .click();
  await dialog().getByLabel("listen port", { exact: true }).fill("18082");
  await dialog().getByRole("button", { name: "Save to draft" }).click();
  await nav("Outbounds").click();
  await page.getByRole("button", { name: "Add outbound", exact: true }).click();
  await dialog().locator("select").first().selectOption("direct");
  await dialog()
    .getByLabel(/^tag\s*\*?$/i)
    .fill("qa-direct");
  await dialog().getByRole("button", { name: "Save to draft" }).click();
  await nav("Routing").click();
  await page.getByRole("button", { name: "Add rule", exact: true }).click();

  await dialog()
    .getByLabel("domain suffix", { exact: true })
    .fill("example.invalid");
  await dialog().getByLabel("Action", { exact: true }).selectOption("route");
  await dialog()
    .getByLabel("outbound", { exact: true })
    .selectOption("qa-direct");
  await dialog().getByRole("button", { name: "Save to draft" }).click();
  await page
    .getByRole("button", { name: "Duplicate rule", exact: true })
    .first()
    .click();
  await dialog()
    .getByLabel("domain suffix", { exact: true })
    .fill("copy.invalid");
  await dialog().getByRole("button", { name: "Save to draft" }).click();
  await page
    .getByRole("button", { name: "Move rule down", exact: true })
    .first()
    .click();
  await nav("Rule Sets").click();
  await page.getByRole("button", { name: "Add rule set", exact: true }).click();
  await dialog().locator("select").first().selectOption("inline");
  await dialog()
    .getByLabel(/^tag\s*\*?$/i)
    .fill("qa-set");
  await dialog()
    .getByRole("button", { name: "Add condition", exact: true })
    .click();
  await dialog()
    .getByLabel("domain suffix", { exact: true })
    .fill("example.invalid");
  await dialog().getByRole("button", { name: "Save to draft" }).click();
  await page.getByRole("button", { name: "Save draft", exact: true }).click();
  await page.getByRole("button", { name: "Validate", exact: true }).click();
  await page
    .getByRole("status")
    .filter({ hasText: "validation passed" })
    .waitFor();
  await nav("Outbounds").click();
  await page
    .getByRole("button", { name: "Delete qa-direct", exact: true })
    .click();
  await page
    .getByRole("alert")
    .filter({ hasText: "Resolve references" })
    .waitFor();
  const draft = await page.evaluate(
    async () => (await (await fetch("/api/config")).json()).draft.config,
  );
  assert.equal(draft.route.rules[0].domain_suffix, "copy.invalid");
  assert.equal(draft.inbounds[0].listen_port, 18082);
  console.log(
    "PASS visual outbound/routing/ruleset editors, persisted ordering/duplicate and dependency protection",
  );
  await nav("JSON Editor").click();
  await page.locator(".monaco-editor").first().waitFor();
  const editor = page.locator(".monaco-editor textarea").first();
  await editor.focus();
  await page.keyboard.press(
    process.platform === "darwin" ? "Meta+A" : "Control+A",
  );
  await page.keyboard.insertText("{");
  await page.getByRole("button", { name: "Save draft", exact: true }).click();
  await page.locator(".notice.error[role=alert]").waitFor();
  assert.deepEqual(
    await page.evaluate(
      async () => (await (await fetch("/api/config")).json()).draft.config,
    ),
    draft,
  );
  await page.reload();
  await nav("JSON Editor").click();
  await page.locator(".monaco-editor").first().waitFor();
  await page.getByRole("button", { name: "View diff", exact: true }).click();
  await dialog().locator(".monaco-diff-editor").waitFor();
  await dialog().getByRole("button", { name: "Keep editing" }).click();
  async function checkEditorAfterDiffDisposal() {
    if (deferEditorIdle) await page.evaluate(() => window.flushTestIdle());
    // Force fresh hover/action-bar consumers after disposing the diff's child
    // services. The old failure otherwise depends on deferred widget timing.
    await page
      .locator(".monaco-editor .view-lines")
      .first()
      .click({ button: "right" });
    await page.getByRole("menu").waitFor();
    await page.keyboard.press("Escape");
    await page.locator(".monaco-editor textarea").first().focus();
    await page.keyboard.press(
      process.platform === "darwin" ? "Meta+f" : "Control+f",
    );
    await page.locator(".monaco-editor .find-widget.visible").waitFor();
    await page.keyboard.press("Escape");
    await page.keyboard.press(
      process.platform === "darwin" ? "Meta+Home" : "Control+Home",
    );
    await page.keyboard.press("ArrowRight");
    await page.keyboard.press("Control+Space");
    await page.locator(".monaco-editor .suggest-widget.visible").waitFor();
    await page.keyboard.press("Escape");
    assert.deepEqual(errors, [], "editor controls must survive diff disposal");
  }
  await checkEditorAfterDiffDisposal();
  await page.getByRole("button", { name: "Live Logs", exact: true }).click();
  await dialog().getByLabel("Search logs").fill("INFO");
  await dialog().getByRole("button", { name: "Pause", exact: true }).click();
  await dialog().getByRole("button", { name: "Resume", exact: true }).click();
  await dialog().getByLabel("Clear visible buffer").click();
  await dialog().getByLabel("Close", { exact: true }).click();
  console.log("PASS locally bundled Monaco diff and logs controls");
  for (let cycle = 0; cycle < 3; cycle++) {
    await nav("JSON Editor").click();
    await page.locator(".monaco-editor").first().waitFor();
    await page.getByRole("button", { name: "View diff", exact: true }).click();
    await dialog().locator(".monaco-diff-editor").waitFor();
    await dialog().getByRole("button", { name: "Keep editing" }).click();
    await checkEditorAfterDiffDisposal();
    await nav("Overview").click();
    assert.equal(
      await page.locator(".monaco-editor").count(),
      0,
      "editor widgets must be disposed after navigation",
    );
    await page.waitForTimeout(100);
  }
  console.log(
    "PASS three Monaco/diff mount-unmount cycles, context menus, find, suggestions and widget cleanup",
  );
  // Polling must detect external edits and prevent stale writes.
  await fs.writeFile(
    configPath,
    JSON.stringify({ ...live, log: { level: "warn" } }),
    { mode: 0o600 },
  );
  await page.getByText(/The live file changed outside Manager/).waitFor();
  assert.equal(
    await page
      .getByRole("button", { name: "Save draft", exact: true })
      .isDisabled(),
    true,
  );
  await page
    .getByRole("button", { name: "Discard draft & use live", exact: true })
    .click();
  console.log("PASS external change conflict and deliberate draft discard");
  await nav("Overview").click();
  await page.getByText("Native API connected", { exact: true }).waitFor();
  await page.screenshot({
    path: path.join(screenshotDir, "overview-desktop.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Light appearance", exact: true })
    .click();
  await page.screenshot({
    path: path.join(screenshotDir, "overview-light.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Dark appearance", exact: true })
    .click();
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(500);
  assert.ok(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
    "mobile page overflows",
  );
  await page.screenshot({
    path: path.join(screenshotDir, "overview-mobile.png"),
  });
  await nav("Inbounds").click();
  await page.getByRole("button", { name: "Add inbound", exact: true }).click();
  assert.ok(await dialog().isVisible());
  await page.screenshot({
    path: path.join(screenshotDir, "editor-mobile.png"),
  });
  await dialog().getByLabel("Close", { exact: true }).click();
  for (const name of [
    "Outbounds",
    "Routing",
    "Rule Sets",
    "JSON Editor",
    "Settings",
  ]) {
    await nav(name).click();
    await page.waitForTimeout(100);
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
      name + " overflows mobile",
    );
  }
  assert.deepEqual(errors, []);
  console.log(
    "PASS desktop/light/all mobile pages and mobile editor; no browser exceptions",
  );
} catch (e) {
  failed = true;
  await diagnostic.failure(page, e);
  console.error("Browser diagnostics:", artifactDir);
  throw e;
} finally {
  try {
    await context?.tracing.stop(
      failed ? { path: path.join(artifactDir, "trace.zip") } : {},
    );
  } catch (error) {
    diagnostic.record("trace-error", { message: error.message });
  }
  await browser?.close();
  manager?.kill("SIGTERM");
  proxy?.kill("SIGTERM");
  await new Promise((r) => setTimeout(r, 500));
  await diagnostic.save();
  await fs.rm(dir, { recursive: true, force: true });
}
