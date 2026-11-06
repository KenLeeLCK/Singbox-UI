import { useEffect, useRef } from "react";
import * as monaco from "monaco-editor";
import { disposeEditor } from "./editorLifecycle";
interface EditorProps {
  height: string;
  language: string;
  theme: string;
  value: string;
  onChange?: (value: string) => void;
  onMount?: (
    editor: monaco.editor.IStandaloneCodeEditor,
    api: typeof monaco,
  ) => void;
  options?: monaco.editor.IStandaloneEditorConstructionOptions;
}
/** Models belong to this component, never to an external React wrapper. */
export default function Editor(props: EditorProps) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
  const model = useRef<monaco.editor.ITextModel | null>(null);
  const current = useRef(props);
  current.current = props;
  useEffect(() => {
    const p = current.current;
    const m = monaco.editor.createModel(p.value, p.language);
    const e = monaco.editor.create(host.current!, {
      editContext: false,
      ...p.options,
      model: m,
      theme: p.theme,
    });
    model.current = m;
    editor.current = e;
    const subscription = m.onDidChangeContent(() =>
      current.current.onChange?.(m.getValue()),
    );
    p.onMount?.(e, monaco);
    return () => {
      disposeEditor(e, [m], subscription);
      editor.current = null;
      model.current = null;
    };
  }, []);
  useEffect(() => {
    if (model.current && model.current.getValue() !== props.value)
      model.current.setValue(props.value);
  }, [props.value]);
  useEffect(() => {
    editor.current?.updateOptions(props.options || {});
  }, [props.options]);
  useEffect(() => {
    monaco.editor.setTheme(props.theme);
  }, [props.theme]);
  return (
    <div
      ref={host}
      style={{ height: props.height, minWidth: 0 }}
      aria-label="JSON configuration editor"
    />
  );
}
interface DiffProps {
  height: string;
  language: string;
  theme: string;
  original: string;
  modified: string;
  options?: monaco.editor.IStandaloneDiffEditorConstructionOptions;
}
export function DiffEditor(props: DiffProps) {
  const host = useRef<HTMLDivElement>(null);
  const editor = useRef<monaco.editor.IStandaloneDiffEditor | null>(null);
  const models = useRef<monaco.editor.IDiffEditorModel | null>(null);
  const current = useRef(props);
  current.current = props;
  useEffect(() => {
    const p = current.current;
    const m = {
      original: monaco.editor.createModel(p.original, p.language),
      modified: monaco.editor.createModel(p.modified, p.language),
    };
    const e = monaco.editor.createDiffEditor(host.current!, {
      editContext: false,
      ...p.options,
      theme: p.theme,
    });
    models.current = m;
    editor.current = e;
    e.setModel(m);
    return () => {
      disposeEditor(e, [m.original, m.modified]);
      editor.current = null;
      models.current = null;
    };
  }, []);
  useEffect(() => {
    if (models.current?.original.getValue() !== props.original)
      models.current?.original.setValue(props.original);
  }, [props.original]);
  useEffect(() => {
    if (models.current?.modified.getValue() !== props.modified)
      models.current?.modified.setValue(props.modified);
  }, [props.modified]);
  useEffect(() => {
    editor.current?.updateOptions(props.options || {});
  }, [props.options]);
  useEffect(() => {
    monaco.editor.setTheme(props.theme);
  }, [props.theme]);
  return (
    <div
      ref={host}
      style={{ height: props.height, minWidth: 0 }}
      aria-label="Configuration comparison"
    />
  );
}
