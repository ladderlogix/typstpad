import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

export default defineConfig({
  plugins: [react(), tailwindcss()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    // wasm files are large; silence the size warning
    chunkSizeWarningLimit: 4000,
  },
  worker: {
    format: "es",
  },
  server: {
    proxy: {
      "/api": { target: "http://localhost:8080", changeOrigin: true },
      "/collab": { target: "http://localhost:8080", ws: true },
      "/healthz": { target: "http://localhost:8080" },
    },
  },
});
