import React from "react";
import ReactDOM from "react-dom/client";
import editorWorker from "monaco-editor/editor/editor.worker?worker";
import jsonWorker from "monaco-editor/language/json/json.worker?worker";
import App from "./App";
import { ErrorBoundary } from "./components/ErrorBoundary";
import "./style.css";
(self as any).MonacoEnvironment = {
  getWorker(_: string, label: string) {
    return label === "json" ? new jsonWorker() : new editorWorker();
  },
};
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <App />
    </ErrorBoundary>
  </React.StrictMode>,
);
