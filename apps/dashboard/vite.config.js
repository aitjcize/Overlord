import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";

// Get API server configuration from environment variables or use defaults
const API_URL = process.env.VITE_API_URL || "http://localhost:9000";

export default defineConfig({
  plugins: [vue()],
  base: "/apps/dashboard", // Keep base as /apps/dashboard for correct asset loading
  resolve: {
    alias: {
      "@": fileURLToPath(new URL("./src", import.meta.url)),
    },
  },
  server: {
    proxy: {
      "/api": {
        target: API_URL,
        changeOrigin: true,
        ws: true, // Enable WebSocket proxy
      },
    },
  },
});
