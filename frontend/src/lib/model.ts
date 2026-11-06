export type Obj = Record<string, any>;
export const pretty = (v: unknown) => JSON.stringify(v, null, 2);
export function resolve(root: Obj, node: Obj = {}, depth = 0): Obj {
  if (depth > 20) return {};
  if (node.$ref)
    return resolve(
      root,
      node.$ref
        .replace(/^#\//, "")
        .split("/")
        .reduce(
          (a: Obj, k: string) => a?.[k.replace(/~1/g, "/").replace(/~0/g, "~")],
          root,
        ),
      depth + 1,
    );
  let out = { ...node, properties: { ...node.properties } };
  for (const sub of node.allOf || []) {
    const s = resolve(root, sub, depth + 1);
    out = { ...out, ...s, properties: { ...out.properties, ...s.properties } };
  }
  return out;
}
export function variants(root: Obj, node: Obj, depth = 0): Obj[] {
  if (depth > 20) return [];
  const n = resolve(root, node);
  const union = n.oneOf || n.anyOf;
  if (!union) return [n];
  const base = { ...n };
  delete base.oneOf;
  delete base.anyOf;
  return union.flatMap((part: Obj) =>
    variants(root, part, depth + 1).map((child) => ({
      ...base,
      ...child,
      properties: { ...base.properties, ...child.properties },
    })),
  );
}
export function sectionSchema(root: Obj, section: string): Obj {
  const props = resolve(root, root).properties || {};
  return section === "inbounds" || section === "outbounds"
    ? resolve(root, props[section]).items || {}
    : resolve(root, resolve(root, props.route).properties?.[section]).items ||
        {};
}
export function types(root: Obj, section: string): string[] {
  return [
    ...new Set(
      variants(root, sectionSchema(root, section)).flatMap((s) => {
        const p = resolve(root, s.properties?.type);
        return p.const ? [p.const] : p.enum || [];
      }),
    ),
  ];
}
export function objectSchema(root: Obj, section: string, type: string): Obj {
  return (
    variants(root, sectionSchema(root, section)).find((s) => {
      const p = resolve(root, s.properties?.type);
      return p.const === type || p.enum?.includes(type);
    }) ||
    variants(root, sectionSchema(root, section))[0] ||
    {}
  );
}
export function collection(c: Obj, section: string): Obj[] {
  return section === "inbounds" || section === "outbounds"
    ? c[section] || []
    : c.route?.[section] || [];
}
export function replaceCollection(c: Obj, section: string, items: Obj[]): Obj {
  return section === "inbounds" || section === "outbounds"
    ? { ...c, [section]: items }
    : { ...c, route: { ...c.route, [section]: items } };
}
export function dependency(c: Obj, tag: string): boolean {
  let found = false;
  const visit = (v: any, k = "") => {
    if (
      [
        "outbound",
        "outbounds",
        "final",
        "detour",
        "default",
        "rule_set",
        "inbound",
      ].includes(k) &&
      (v === tag || (Array.isArray(v) && v.includes(tag)))
    )
      found = true;
    if (v && typeof v === "object")
      Object.entries(v).forEach(([key, x]) => visit(x, key));
  };
  visit(c);
  return found;
}
export function bytes(n?: number | null): string {
  if (n == null || !Number.isFinite(n)) return "—";
  if (n === 0) return "0 B";
  const i = Math.min(4, Math.max(0, Math.floor(Math.log(n) / Math.log(1024))));
  return `${(n / 1024 ** i).toFixed(i ? 1 : 0)} ${["B", "KB", "MB", "GB", "TB"][i]}`;
}
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api(path: string, method = "GET", body?: unknown) {
  const r = await fetch("/api/" + path, {
    method,
    // Deployment may include validation, restart, health checks and rollback.
    signal: AbortSignal.timeout(
      method === "GET" || path === "login" ? 15000 : 120000,
    ),
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Requested-With": "sing-box-manager",
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!r.ok) {
    const detail = await r.json().catch(() => null);
    const message =
      r.status === 401
        ? path === "login"
          ? "Invalid access token."
          : "Your session expired. Sign in again."
        : detail?.error ||
          `Request failed (${r.status}). Check Manager connectivity and retry.`;
    throw new ApiError(r.status, message);
  }
  const value = await r.json().catch(() => {
    throw new Error(
      "Manager returned invalid JSON. Check the proxy target and retry.",
    );
  });
  return value;
}

// RuleAction is a union; remove every possible action field from WHEN controls.
// The full rule object is retained by each update, so hidden fields survive.
export function matchSchema(root: Obj, type: string): Obj {
  const rule = objectSchema(root, "rules", type);
  const actionFields = new Set(
    variants(root, root.$defs?.RuleAction || {}).flatMap((action) =>
      Object.keys(action.properties || {}),
    ),
  );
  return {
    ...rule,
    properties: Object.fromEntries(
      Object.entries(rule.properties || {}).filter(
        ([name]) => !actionFields.has(name),
      ),
    ),
  };
}

export function tagNames(value: Obj): string[] {
  return (
    Array.isArray(value.tag) ? value.tag : value.tag ? [value.tag] : []
  ).filter((tag: unknown): tag is string => typeof tag === "string");
}
