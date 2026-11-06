import { afterEach, expect, it, vi } from "vitest";
import { api, ApiError } from "./model";
afterEach(() => vi.unstubAllGlobals());
it("classifies plain text 401 responses by status", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response("session expired", { status: 401 })),
  );
  await expect(api("config")).rejects.toMatchObject({
    status: 401,
    message: "Your session expired. Sign in again.",
  });
});
it("preserves actionable backend validation messages", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ error: "live configuration changed" }), {
        status: 400,
      }),
    ),
  );
  await expect(api("validate", "POST", {})).rejects.toThrow(
    "live configuration changed",
  );
});
it("rejects HTML fallback responses instead of treating them as configuration", async () => {
  vi.stubGlobal(
    "fetch",
    vi.fn().mockResolvedValue(new Response("<html>proxy fallback</html>")),
  );
  await expect(api("config")).rejects.toThrow("invalid JSON");
});
it("uses same-origin credentials, CSRF header and a bounded request", async () => {
  const fetch = vi.fn().mockResolvedValue(new Response("{}"));
  vi.stubGlobal("fetch", fetch);
  await api("config");
  expect(fetch).toHaveBeenCalledWith(
    "/api/config",
    expect.objectContaining({
      credentials: "same-origin",
      signal: expect.any(AbortSignal),
      headers: expect.objectContaining({
        "X-Requested-With": "sing-box-manager",
      }),
    }),
  );
  expect(new ApiError(401, "expired")).toBeInstanceOf(Error);
});

it("reports temporary login throttling with retry guidance", async () => {
  vi.stubGlobal(
    "fetch",
    vi
      .fn()
      .mockResolvedValue(
        new Response("too many attempts", {
          status: 429,
          headers: { "Retry-After": "37" },
        }),
      ),
  );
  await expect(api("login", "POST", { token: "test" })).rejects.toMatchObject({
    status: 429,
    message: "Too many login attempts. Try again in 37 seconds.",
  });
});
