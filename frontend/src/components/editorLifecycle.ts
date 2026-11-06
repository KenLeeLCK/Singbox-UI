/** Editors own subscriptions to models: detach and dispose the widget first. */
export function disposeEditor(
  editor: { setModel(model: null): void; dispose(): void },
  models: { dispose(): void }[],
  subscription?: { dispose(): void },
) {
  subscription?.dispose();
  editor.setModel(null);
  editor.dispose();
  for (const model of models) model.dispose();
}
