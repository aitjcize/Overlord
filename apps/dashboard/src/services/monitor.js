export class MonitorService {
  constructor() {
    this.eventHandlers = new Map();
    this.connect();
  }

  connect() {
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${window.location.host}/api/monitor`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onmessage = (event) => {
      const message = JSON.parse(event.data);
      const handlers = this.eventHandlers.get(message.event) || [];
      const data =
        message.data && message.data.length > 0 ? message.data[0] : null;
      handlers.forEach((handler) => handler(data));
    };

    this.ws.onclose = () => {
      // Reconnect after 1 second
      setTimeout(() => this.connect(), 1000);
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
      const index = handlers.indexOf(callback);
      if (index > -1) {
        handlers.splice(index, 1);
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
