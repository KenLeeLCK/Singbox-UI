import fs from "node:fs/promises";
import path from "node:path";

// Only isolated fixtures are recorded. Never point this suite at a live Manager.
export async function diagnostics(directory) {
  await fs.mkdir(directory, { recursive: true });
  const events = [];
  const pending = new Set();
  const record = (type, details) =>
    events.push({ time: new Date().toISOString(), type, ...details });
  function process(name, child) {
    for (const channel of ["stdout", "stderr"])
      child[channel]?.on("data", (data) =>
        record("process", { name, channel, text: data.toString() }),
      );
    child.on("error", (error) =>
      record("process-error", { name, message: error.message }),
    );
    child.on("exit", (code, signal) =>
      record("process-exit", { name, code, signal }),
    );
  }
  function page(page) {
    page.on("console", (message) => {
      const event = {
        level: message.type(),
        text: message.text(),
        location: message.location(),
      };
      record("console", event);
      // console.text() and Error.message can both be empty for AggregateError.
      const task = Promise.all(
        message.args().map((arg) =>
          arg
            .evaluate((value) => {
              function serialize(value) {
                if (value instanceof Error)
                  return {
                    name: value.name,
                    message: value.message,
                    stack: value.stack,
                    errors: value.errors?.map(serialize),
                    cause: value.cause ? serialize(value.cause) : undefined,
                  };
                return value;
              }
              return serialize(value);
            })
            .catch((error) => ({ unavailable: error.message })),
        ),
      ).then((args) => {
        record("console-arguments", { ...event, args });
      });
      pending.add(task);
      task.finally(() => pending.delete(task));
    });
    page.on("pageerror", (error) =>
      record("exception", {
        name: error.name,
        message: error.message,
        stack: error.stack,
      }),
    );
    page.on("requestfailed", (request) =>
      record("network-failure", {
        url: request.url(),
        method: request.method(),
        error: request.failure()?.errorText,
      }),
    );
    page.on("response", (response) => {
      const type = response.request().resourceType();
      if (
        response.status() >= 400 ||
        ["script", "stylesheet", "document"].includes(type)
      )
        record("response", {
          url: response.url(),
          status: response.status(),
          resourceType: type,
          contentType: response.headers()["content-type"],
        });
    });
    page.on("worker", (worker) => record("worker", { url: worker.url() }));
    page.on("crash", () => record("page-crash", {}));
  }
  async function failure(page, error) {
    record("test-failure", { message: error.message, stack: error.stack });
    if (page && !page.isClosed()) {
      for (const [name, action] of [
        [
          "screenshot",
          () =>
            page.screenshot({
              path: path.join(directory, "failure.png"),
              fullPage: true,
              mask: [page.locator(".monaco-editor")],
            }),
        ],
        [
          "DOM",
          async () =>
            fs.writeFile(
              path.join(directory, "failure.html"),
              await page.content(),
            ),
        ],
        [
          "navigation",
          async () =>
            record(
              "navigation-state",
              await page.evaluate(() => ({
                url: location.href,
                readyState: document.readyState,
                rootChildren:
                  document.getElementById("root")?.childElementCount,
                buttons: [...document.querySelectorAll("nav button")].map(
                  (button) => {
                    const rect = button.getBoundingClientRect(),
                      style = getComputedStyle(button);
                    return {
                      text: button.textContent,
                      disabled: button.disabled,
                      display: style.display,
                      visibility: style.visibility,
                      rect: rect.toJSON(),
                      hit: document
                        .elementFromPoint(
                          rect.x + rect.width / 2,
                          rect.y + rect.height / 2,
                        )
                        ?.outerHTML.slice(0, 300),
                    };
                  },
                ),
              })),
            ),
        ],
      ]) {
        try {
          await action();
        } catch (error) {
          record("diagnostic-error", { name, message: error.message });
        }
      }
    }
  }
  async function save() {
    await Promise.allSettled([...pending]);
    await fs.writeFile(
      path.join(directory, "events.json"),
      JSON.stringify(events, null, 2),
    );
  }
  return { process, page, failure, save, record };
}
