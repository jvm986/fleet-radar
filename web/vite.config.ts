import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";

// The backend serves only the stream, so everything under /api is proxied to it in development. There
// is no other endpoint: the client requests nothing beyond the GET that opens the stream
// (ADR-0005 §5.4).
const proxy = { "/api": { target: "http://localhost:8080" } };

export default defineConfig({
  plugins: [react()],
  server: { proxy },
  preview: { proxy },
});
