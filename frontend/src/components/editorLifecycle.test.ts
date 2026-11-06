import { it, expect } from "vitest";
import { disposeEditor } from "./editorLifecycle";
it("detaches diff models and disposes editor before owned models", () => {
  const calls: string[] = [];
  disposeEditor(
    {
      setModel: (m) => {
        expect(m).toBeNull();
        calls.push("detach");
      },
      dispose: () => calls.push("editor"),
    },
    [
      { dispose: () => calls.push("original") },
      { dispose: () => calls.push("modified") },
    ],
    { dispose: () => calls.push("subscription") },
  );
  expect(calls).toEqual([
    "subscription",
    "detach",
    "editor",
    "original",
    "modified",
  ]);
});
