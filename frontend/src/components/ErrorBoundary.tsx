import { Component, type ReactNode } from "react";

export class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };
  static getDerivedStateFromError() {
    return { failed: true };
  }
  componentDidCatch() {
    // Avoid logging configuration values or credentials from exception messages.
    console.error("Manager rendering failed. Reload to recover.");
  }
  render() {
    if (this.state.failed)
      return (
        <div className="login glass" role="alert">
          <h1>Manager could not display this page</h1>
          <p>
            Reload to recover. Unsaved changes may be lost. If this continues,
            check the configuration and report the failing page.
          </p>
          <button className="primary" onClick={() => location.reload()}>
            Reload Manager
          </button>
        </div>
      );
    return this.props.children;
  }
}
