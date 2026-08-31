import React from "react";
import ReactDOM from "react-dom/client";

import App from "@/App";
import { ApplicationErrorBoundary } from "@/components/ApplicationErrorBoundary";
import "@/index.css";

ReactDOM.createRoot(document.getElementById("root") as HTMLElement).render(
  <React.StrictMode>
    <ApplicationErrorBoundary><App /></ApplicationErrorBoundary>
  </React.StrictMode>
);
