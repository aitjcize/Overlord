import { defineStore } from "pinia";
import { monitorService } from "@/services/monitor";
import { apiService } from "@/services/api";

export const useClientStore = defineStore("clients", {
  state: () => ({
    clients: new Map(),
    recentClients: [],
    terminals: new Map(),
    fixtures: new Map(),
    cameras: new Map(),
    activeClientId: null,
    filterPattern: "",
  }),

  actions: {
    async loadInitialClients() {
      try {
        const clients = await apiService.getClients();

        // Load clients in sequence to avoid overwhelming the server
        for (const client of clients) {
          try {
            const properties = await apiService.getClientProperties(client.mid);
            client.properties = properties;

            // Add to clients map but don't add to recent list for initial load
            if (!client || !client.mid) {
              console.error("Invalid client object:", client);
              return;
            }
            client.lastSeen = Date.now();
            this.clients.set(client.mid, client);
          } catch (error) {
            console.error(
              `Failed to load properties for client ${client.mid}:`,
              error,
            );
          }
        }
      } catch (error) {
        console.error("Failed to load initial clients:", error);
      }
    },

    addClient(client) {
      // Ensure we have a valid client object with a mid
      if (!client || !client.mid) {
        console.error("Invalid client object:", client);
        return;
      }

      // Add lastSeen timestamp for new clients
      client.lastSeen = Date.now();
      this.clients.set(client.mid, client);

      // Only add to recent clients if this is a new connection (from WebSocket)
      // Initial load from REST API should not affect recent clients
      if (monitorService.isConnected) {
        this.recentClients = [
          client,
          ...this.recentClients.filter((c) => c.mid !== client.mid),
        ].slice(0, 5);
      }
    },

    removeClient(mid) {
      this.clients.delete(mid);
      // Remove from recent clients when disconnected
      this.recentClients = this.recentClients.filter((c) => c.mid !== mid);
      this.removeFixture(mid);
    },

    addTerminal(id, client) {
      if (!client || !client.mid) {
        console.error("Invalid client object:", client);
        return null;
      }

      console.log("Adding new terminal", { id, clientMid: client.mid });

      const terminal = {
        id,
        mid: client.mid,
        sessionId: client.mid,
        clientName: client.name || client.mid,
        isMinimized: false,
        isFocused: true,
        zIndex: 0,
      };

      this.terminals.set(id, terminal);
      console.log("Terminal added", { id, terminal });

      // Explicitly minimize all other terminals when adding a new one
      const terminals = Array.from(this.terminals.values());
      terminals.forEach((t) => {
        if (t.id !== id) {
          t.isMinimized = true;
          t.isFocused = false;
        }
      });

      return terminal;
    },

    removeTerminal(id) {
      this.terminals.delete(id);
    },

    minimizeAllTerminalsExcept(activeId, shouldMinimizeActive = false) {
      console.log("minimizeAllTerminalsExcept called", {
        activeId,
        shouldMinimizeActive,
      });

      const terminals = Array.from(this.terminals.values());
      terminals.forEach((terminal) => {
        const isActive = terminal.id === activeId;
        const shouldMinimize = isActive ? shouldMinimizeActive : true;

        console.log("Setting terminal state:", {
          id: terminal.id,
          isActive,
          shouldMinimize,
          beforeState: terminal,
        });

        terminal.isMinimized = shouldMinimize;
        terminal.isFocused = !shouldMinimize && isActive;

        console.log("Terminal state after update:", terminal);
      });

      // Dispatch a custom event to force UI updates
      setTimeout(() => {
        window.dispatchEvent(
          new CustomEvent("terminal-state-changed", { detail: { terminals } }),
        );
      }, 0);
    },

    maximizeTerminal(id) {
      console.log("maximizeTerminal called", { id });

      const terminals = Array.from(this.terminals.values());
      terminals.forEach((terminal) => {
        const shouldMinimize = terminal.id !== id;

        console.log("Setting terminal state:", {
          id: terminal.id,
          shouldMinimize,
          beforeState: terminal,
        });

        terminal.isMinimized = shouldMinimize;
        terminal.isFocused = !shouldMinimize;

        console.log("Terminal state after update:", terminal);
      });
    },

    addFixture(client) {
      if (this.fixtures.has(client.mid)) return;
      this.fixtures.set(client.mid, client);

      // Limit to 8 fixtures
      if (this.fixtures.size > 8) {
        const firstKey = this.fixtures.keys().next().value;
        this.fixtures.delete(firstKey);
      }
    },

    removeFixture(mid) {
      this.fixtures.delete(mid);
    },

    addCamera(id, client) {
      this.cameras.set(id, { id, client });
    },

    removeCamera(id) {
      this.cameras.delete(id);
    },

    setFilterPattern(pattern) {
      this.filterPattern = pattern;
    },
  },

  getters: {
    activeRecentClients() {
      // Only return clients that are still connected
      return this.recentClients.filter((client) =>
        this.clients.has(client.mid),
      );
    },

    filteredClients() {
      if (!this.filterPattern) return Array.from(this.clients.values());
      const pattern = this.filterPattern.toLowerCase();
      return Array.from(this.clients.values()).filter((client) =>
        client.mid.toLowerCase().includes(pattern),
      );
    },
  },
});

// For backward compatibility during migration
export const clientStore = {
  get clients() {
    return useClientStore().clients;
  },
  get recentClients() {
    return useClientStore().recentClients;
  },
  get terminals() {
    return useClientStore().terminals;
  },
  get fixtures() {
    return useClientStore().fixtures;
  },
  get cameras() {
    return useClientStore().cameras;
  },
  get activeClientId() {
    return useClientStore().activeClientId;
  },
  get filterPattern() {
    return useClientStore().filterPattern;
  },
  get activeRecentClients() {
    return useClientStore().activeRecentClients;
  },
  get filteredClients() {
    return useClientStore().filteredClients;
  },

  loadInitialClients: () => useClientStore().loadInitialClients(),
  addClient: (client) => useClientStore().addClient(client),
  removeClient: (mid) => useClientStore().removeClient(mid),
  addTerminal: (id, client) => useClientStore().addTerminal(id, client),
  removeTerminal: (id) => useClientStore().removeTerminal(id),
  minimizeAllTerminalsExcept: (activeId, shouldMinimizeActive) =>
    useClientStore().minimizeAllTerminalsExcept(activeId, shouldMinimizeActive),
  maximizeTerminal: (id) => useClientStore().maximizeTerminal(id),
  addFixture: (client) => useClientStore().addFixture(client),
  removeFixture: (mid) => useClientStore().removeFixture(mid),
  addCamera: (id, client) => useClientStore().addCamera(id, client),
  removeCamera: (id) => useClientStore().removeCamera(id),
  setFilterPattern: (pattern) => useClientStore().setFilterPattern(pattern),
};
