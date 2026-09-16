import react from "@vitejs/plugin-react";
import { defineConfig } from "vite";
import tsconfigPaths from "vite-tsconfig-paths";

export default defineConfig({
  plugins: [react(), tsconfigPaths()],
  build: { outDir: "build/client", emptyOutDir: true },
  server: {
    port: 3000,
    proxy: Object.fromEntries([
      "/api", "/v1", "/graphql", "/threads", "/sse",
      "^/auth/(?!forgot-password|reset-password|verify-otp)",
    ].map(path => [path, {
      target: process.env.THREADIFY_DEV_ENGINE_URL || "http://127.0.0.1:8081",
      ws: true,
    }])),
  },
});
