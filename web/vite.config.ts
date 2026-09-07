import react from "@vitejs/plugin-react";
// From vitest/config rather than vite, so the test settings below are typed.
import { defineConfig } from "vitest/config";

// The backend serves only the stream, so everything under /api is proxied to it in development. There
// is no other endpoint: the client requests nothing beyond the GET that opens the stream
// (ADR-0005 §5.4).
const proxy = { "/api": { target: "http://localhost:8080" } };

export default defineConfig({
  plugins: [react()],
  server: { proxy },
  // Tests cover the logic that fails silently: filter composition, the disconnected watchdog, and what
  // persists. Rendering fails visibly, so it is not tested here (ADR-0009 §9.1).
  test: { environment: "jsdom" },
  preview: { proxy },
});
