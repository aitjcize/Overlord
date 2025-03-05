import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import { fileURLToPath, URL } from "node:url";

// Get API server configuration from environment variables or use defaults
const API_HOST = process.env.VITE_API_HOST || "localhost";
const API_PORT = process.env.VITE_API_PORT || "9000";
const API_SERVER = `http://${API_HOST}:${API_PORT}`;

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
        target: API_SERVER,
        changeOrigin: true,
        ws: true, // Enable WebSocket proxy
      },
    },
  },
});
