// Get API server configuration from environment variables or use defaults
const API_HOST = import.meta.env.VITE_API_HOST || window.location.hostname;
const API_PORT = import.meta.env.VITE_API_PORT || "9000";
const API_PROTOCOL = window.location.protocol === "https:" ? "https:" : "http:";
const WS_PROTOCOL = window.location.protocol === "https:" ? "wss:" : "ws:";

export const config = {
  apiUrl: `${API_PROTOCOL}//${API_HOST}:${API_PORT}`,
  wsUrl: `${WS_PROTOCOL}//${API_HOST}:${API_PORT}`,
};
