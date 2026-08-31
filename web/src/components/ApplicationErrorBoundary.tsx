import React from "react";

interface ApplicationErrorBoundaryProps {
  children: React.ReactNode;
}

interface ApplicationErrorBoundaryState {
  failed: boolean;
}

export class ApplicationErrorBoundary extends React.Component<ApplicationErrorBoundaryProps, ApplicationErrorBoundaryState> {
  state: ApplicationErrorBoundaryState = { failed: false };

  static getDerivedStateFromError(): ApplicationErrorBoundaryState {
    return { failed: true };
  }

  componentDidCatch(_error: Error, _info: React.ErrorInfo) {
    // Avoid browser console output because runtime context can contain identifiers.
  }

  render() {
    if (!this.state.failed) return this.props.children;
    const zh = navigator.language.toLowerCase().startsWith("zh");
    return (
      <main className="flex min-h-screen items-center justify-center bg-slate-50 px-5 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
        <section className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-8 text-center shadow-xl shadow-slate-200/40 dark:border-slate-800 dark:bg-slate-900 dark:shadow-black/20">
          <div className="text-5xl font-extrabold text-indigo-600 dark:text-indigo-300">500</div>
          <h1 className="mt-4 text-xl font-bold">{zh ? "页面暂时不可用" : "This page is temporarily unavailable"}</h1>
          <p className="mt-3 text-sm leading-6 text-slate-600 dark:text-slate-400">{zh ? "页面发生意外错误。请重试；若问题持续，请联系平台管理员。" : "An unexpected error occurred. Retry the page or contact your platform administrator if it persists."}</p>
          <button type="button" onClick={() => window.location.reload()} className="mt-6 inline-flex h-10 items-center justify-center rounded-xl bg-indigo-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-indigo-500">
            {zh ? "重新加载" : "Reload"}
          </button>
        </section>
      </main>
    );
  }
}
