import React from "react";
import ReactDOM from "react-dom/client";
import { App } from "./app";
import { GlobalErrorBoundary } from "@/shared/components/ErrorBoundary";
import "./App.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <GlobalErrorBoundary>
      <App />
    </GlobalErrorBoundary>
  </React.StrictMode>,
);
