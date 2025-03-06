import { useAuthStore } from "@/stores/authStore";

export class MonitorService {
  constructor() {
    this.eventHandlers = new Map();
    this.ws = null;
    this.isStarted = false;
    this.reconnectTimer = null;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 5; // Maximum number of reconnect attempts
  }

  start() {
    // Only start if not already started
    if (this.isStarted) return;

    const authStore = useAuthStore();

    // Only connect if authenticated
    if (authStore.isAuthenticated) {
      this.isStarted = true;
      this.reconnectAttempts = 0; // Reset reconnect attempts
      this.connect();
    } else {
      console.log("Monitor service not started: User not authenticated");
    }
  }

  stop() {
    this.isStarted = false;

    // Clear any pending reconnect timer
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }

    // Close WebSocket if it exists
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  connect() {
    // Don't connect if not started or already connected
    if (!this.isStarted || (this.ws && this.ws.readyState === WebSocket.OPEN)) {
      return;
    }

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

    // Get auth token
    const authStore = useAuthStore();
    const token = authStore.token;

    if (!token) {
      console.error("Monitor service: No authentication token available");
      return;
    }

    // Create WebSocket URL
    // Note: For WebSockets, we can't set custom headers directly
    // The server is configured to check both the Authorization header and query parameter
    // Since we can't set headers in WebSocket, we'll use the query parameter approach
    const wsUrl = `${protocol}//${window.location.host}/api/monitor?token=${encodeURIComponent(token)}`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        const handlers = this.eventHandlers.get(message.event) || [];
        const data =
          message.data && message.data.length > 0 ? message.data[0] : null;
        handlers.forEach((handler) => handler(data));
      } catch (error) {
        console.error("Error processing WebSocket message:", error);
      }
    };

    this.ws.onclose = (event) => {
      console.log(`WebSocket closed with code: ${event.code}`);

      // Check for authentication failure (code 1008 is Policy Violation, often used for auth errors)
      if (event.code === 1008 || event.code === 1001) {
        this.reconnectAttempts++;
        console.log(
          `Reconnect attempt ${this.reconnectAttempts} of ${this.maxReconnectAttempts}`,
        );

        // If we've exceeded max reconnect attempts, assume authentication has failed
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
          console.error(
            "WebSocket authentication failed after multiple attempts",
          );

          // Log out the user
          const authStore = useAuthStore();
          if (authStore.isAuthenticated) {
            console.log("WebSocket authentication failed. Logging out...");
            authStore.logout();

            // Force page refresh to show login screen
            window.location.reload();
            return;
          }
        }
      }

      // Only reconnect if service is still started
      if (this.isStarted) {
        // Reconnect after 1 second
        this.reconnectTimer = setTimeout(() => this.connect(), 1000);
      }
    };

    this.ws.onerror = (error) => {
      console.error("WebSocket error:", error);
    };
  }

  on(event, callback) {
    if (!this.eventHandlers.has(event)) {
      this.eventHandlers.set(event, []);
    }
    this.eventHandlers.get(event).push(callback);

    // Return unsubscribe function
    return () => {
      const handlers = this.eventHandlers.get(event);
      if (handlers) {
        const index = handlers.indexOf(callback);
        if (index > -1) {
          handlers.splice(index, 1);
        }
      }
    };
  }

  // Helper method to check connection status
  get isConnected() {
    return this.ws && this.ws.readyState === WebSocket.OPEN;
  }
}

// Create a singleton instance
export const monitorService = new MonitorService();
