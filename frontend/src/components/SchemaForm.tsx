import { useId, useState } from "react";
import { Obj, resolve, variants, pretty, tagNames } from "../lib/model";
export function SchemaForm({
  root,
  schema,
  value,
  onChange,
  depth = 0,
  config = {},
}: {
  root: Obj;
  schema: Obj;
  value: Obj;
  onChange: (v: Obj) => void;
  depth?: number;
  config?: Obj;
}) {
  const s = resolve(root, schema);
  return (
    <div className="form-grid">
      {Object.entries(s.properties || {})
        .filter(([k]) => k !== "type")
        .map(([k, raw]) => (
          <Field
            key={k}
            name={k}
            root={root}
            schema={raw as Obj}
            value={value[k]}
            required={s.required?.includes(k)}
            depth={depth}
            config={config}
            onChange={(v) => {
              const copy = { ...value };
              if (v === undefined) delete copy[k];
              else copy[k] = v;
              onChange(copy);
            }}
          />
        ))}
    </div>
  );
}
function Field({
  name,
  root,
  schema,
  value,
  onChange,
  depth,
  config,
  required,
}: {
  name: string;
  root: Obj;
  schema: Obj;
  value: any;
  onChange: (v: any) => void;
  depth: number;
  config: Obj;
  required?: boolean;
}) {
  let s = resolve(root, schema);
  const opts = variants(root, schema);
  if (opts.length > 1)
    s =
      opts.find(
        (x) => x.type === (Array.isArray(value) ? "array" : typeof value),
      ) ||
      opts.find((x) => x.type === "string") ||
      opts[0];
  const [text, setText] = useState<string | undefined>();
  const [error, setError] = useState("");
  const controlId = useId();
  const label = name.replaceAll("_", " ");
  const enums = s.enum || (s.const ? [s.const] : undefined);
  const refs = ["outbound", "detour", "final", "default", "outbounds"].includes(
    name,
  )
    ? (
        (s["x-tag-reference"] === "inbound"
          ? config.inbounds
          : config.outbounds) || []
      ).flatMap(tagNames)
    : name === "rule_set"
      ? (config.route?.rule_set || []).flatMap(tagNames)
      : name === "inbound"
        ? (config.inbounds || []).flatMap(tagNames)
        : null;
  const edit = (v: string) => {
    setText(v);
    try {
      onChange(v ? JSON.parse(v) : undefined);
      setError("");
    } catch {
      setError("Enter valid JSON");
    }
  };
  let input;
  if ((refs || enums) && s.type !== "array")
    input = (
      <select
        id={controlId}
        aria-label={label}
        value={value ?? ""}
        onChange={(e) =>
          onChange(
            e.target.value === ""
              ? undefined
              : s.type === "integer" || s.type === "number"
                ? Number(e.target.value)
                : e.target.value,
          )
        }
      >
        <option value="">Not set</option>
        {value !== undefined && !(refs || enums).includes(value) && (
          <option value={value}>{String(value)} (existing)</option>
        )}
        {(refs || enums).map((x: string) => (
          <option key={x} value={x}>
            {x}
          </option>
        ))}
      </select>
    );
  else if (refs && s.type === "array")
    input = (
      <div className="checks">
        {refs.map((x: string) => (
          <label key={x}>
            <input
              type="checkbox"
              checked={(value || []).includes(x)}
              onChange={(e) =>
                onChange(
                  e.target.checked
                    ? [...(value || []), x]
                    : (value || []).filter((v: string) => v !== x),
                )
              }
            />
            {x}
          </label>
        ))}
      </div>
    );
  else if (s.type === "boolean")
    input = (
      <select
        id={controlId}
        aria-label={label}
        value={value === undefined ? "" : String(value)}
        onChange={(e) =>
          onChange(
            e.target.value === "" ? undefined : e.target.value === "true",
          )
        }
      >
        <option value="">Default</option>
        <option value="true">Enabled</option>
        <option value="false">Disabled</option>
      </select>
    );
  else if (
    (s.type === "object" || Object.keys(s.properties || {}).length) &&
    depth < 2
  )
    input = (
      <details className="nested">
        <summary>{value ? "Configured" : "Optional settings"}</summary>
        <SchemaForm
          root={root}
          schema={s}
          value={value || {}}
          onChange={onChange}
          config={config}
          depth={depth + 1}
        />
        <button
          type="button"
          className="quiet"
          onClick={() => onChange(undefined)}
        >
          Remove settings
        </button>
      </details>
    );
  else if (
    s.type === "array" &&
    (resolve(root, s.items).type === "object" || name === "rules") &&
    depth < 6
  )
    input = (
      <div className="array-fields">
        {(value || []).map((v: Obj, i: number) => {
          const choices = variants(root, s.items);
          const itemSchema =
            choices.find((x) => x.properties?.type?.const === v.type) ||
            choices.find((x) => !x.properties?.type?.const) ||
            choices[0] ||
            s.items;
          return (
            <div className="nested" key={i}>
              {name === "rules" && (
                <label className="field">
                  <span>Condition structure</span>
                  <select
                    aria-label="Condition structure"
                    value={v.type || ""}
                    onChange={(e) => {
                      const next = { ...v };
                      if (e.target.value) next.type = e.target.value;
                      else delete next.type;
                      onChange(
                        value.map((old: Obj, j: number) =>
                          j === i ? next : old,
                        ),
                      );
                    }}
                  >
                    <option value="">Standard match</option>
                    <option value="logical">Logical group</option>
                  </select>
                </label>
              )}
              <SchemaForm
                root={root}
                schema={itemSchema}
                value={v}
                onChange={(next) =>
                  onChange(
                    value.map((old: Obj, j: number) => (j === i ? next : old)),
                  )
                }
                depth={depth + 1}
                config={config}
              />
              <button
                type="button"
                className="quiet"
                onClick={() =>
                  onChange(value.filter((_: Obj, j: number) => j !== i))
                }
              >
                Remove item
              </button>
            </div>
          );
        })}
        <button type="button" onClick={() => onChange([...(value || []), {}])}>
          Add {name === "rules" ? "condition" : "item"}
        </button>
      </div>
    );
  else if (s.type === "array" || s.type === "object")
    input = (
      <>
        <textarea
          id={controlId}
          aria-label={label}
          aria-invalid={!!error}
          value={text ?? (value === undefined ? "" : pretty(value))}
          placeholder={s.type === "array" ? '["value"]' : "{}"}
          onChange={(e) => {
            edit(e.target.value);
            try {
              if (e.target.value) JSON.parse(e.target.value);
              e.target.setCustomValidity("");
            } catch {
              e.target.setCustomValidity(
                "Enter valid JSON before saving this entry",
              );
            }
          }}
        />
        {error && <small className="error">{error}</small>}
      </>
    );
  else
    input = (
      <input
        id={controlId}
        aria-label={label}
        type={
          s.type === "integer" || s.type === "number"
            ? "number"
            : /password|secret|private_key/.test(name)
              ? "password"
              : "text"
        }
        value={value ?? ""}
        min={s.minimum}
        max={s.maximum}
        placeholder="Not set"
        onChange={(e) =>
          onChange(
            e.target.value === ""
              ? undefined
              : s.type === "integer" || s.type === "number"
                ? Number(e.target.value)
                : e.target.value,
          )
        }
      />
    );
  const complex =
    s.type === "object" ||
    s.type === "array" ||
    Object.keys(s.properties || {}).length > 0;
  const title = (
    <>
      {label}
      {required && <em> *</em>}
    </>
  );
  return complex ? (
    <fieldset className="field wide">
      <legend>{title}</legend>
      {input}
      {s.description && <small>{s.description}</small>}
    </fieldset>
  ) : (
    <div className="field">
      <label htmlFor={controlId}>{title}</label>
      {input}
      {s.description && <small>{s.description}</small>}
    </div>
  );
}
