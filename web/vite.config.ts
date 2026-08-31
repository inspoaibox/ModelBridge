import { fileURLToPath, URL } from "node:url";

import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import { defineConfig } from "vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      "/healthz": "http://127.0.0.1:8080",
      "/admin": "http://127.0.0.1:8080",
      "/console": "http://127.0.0.1:8080",
      "/public": "http://127.0.0.1:8080",
      "/auth": "http://127.0.0.1:8080",
      "/v1": "http://127.0.0.1:8080",
    },
  },
  build: {
    outDir: "dist",
    emptyOutDir: true,
    chunkSizeWarningLimit: 800,
  },
});
