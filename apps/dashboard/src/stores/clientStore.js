import { defineStore } from "pinia";
import { ref, computed } from "vue";
import { apiService } from "@/services/api";
import { monitorService } from "@/services/monitor";

export const useClientStore = defineStore("clients", () => {
  // State
  const clients = ref({});
  const recentClients = ref([]);
  const fixtures = ref({});
  const cameras = ref({});
  const activeClientId = ref(null);
  const filterPattern = ref("");
  const isUpgrading = ref(false);

  // Getters
  const clientsArray = computed(() => Object.values(clients.value));

  const activeRecentClients = computed(() => {
    // Only return clients that are still connected
    return recentClients.value.filter((client) => clients.value[client.mid]);
  });

  const filteredClients = computed(() => {
    if (!filterPattern.value) return clientsArray.value;
    const pattern = filterPattern.value.toLowerCase();
    return clientsArray.value.filter((client) =>
      client.mid.toLowerCase().includes(pattern),
    );
  });

  // Actions
  async function loadInitialClients() {
    try {
      const clientsList = await apiService.getClients();

      // Load clients in sequence to avoid overwhelming the server
      for (const client of clientsList) {
        try {
          const properties = await apiService.getClientProperties(client.mid);
          client.properties = properties;

          // Add to clients map but don't add to recent list for initial load
          if (!client || !client.mid) {
            console.error("Invalid client object:", client);
            continue;
          }

          client.lastSeen = Date.now();
          clients.value[client.mid] = client;
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
  }

  function addClient(client) {
    // Ensure we have a valid client object with a mid
    if (!client || !client.mid) {
      console.error("Invalid client object:", client);
      return;
    }

    // Add lastSeen timestamp for new clients
    client.lastSeen = Date.now();
    clients.value[client.mid] = client;

    // Only add to recent clients if this is a new connection (from WebSocket)
    // Initial load from REST API should not affect recent clients
    if (monitorService.isConnected) {
      // Filter out the client if it already exists in the recent list
      const filteredRecent = recentClients.value.filter(
        (c) => c.mid !== client.mid,
      );
      // Add to the beginning of the list and limit to 5 items
      recentClients.value = [client, ...filteredRecent].slice(0, 5);
    }
  }

  function removeClient(mid) {
    delete clients.value[mid];
    // Remove from recent clients when disconnected
    recentClients.value = recentClients.value.filter((c) => c.mid !== mid);
    removeFixture(mid);
  }

  function addFixture(client) {
    if (fixtures.value[client.mid]) return;

    fixtures.value[client.mid] = client;

    // Limit to 8 fixtures
    const fixtureKeys = Object.keys(fixtures.value);
    if (fixtureKeys.length > 8) {
      delete fixtures.value[fixtureKeys[0]];
    }
  }

  function removeFixture(mid) {
    delete fixtures.value[mid];
  }

  function addCamera(id, client) {
    cameras.value[id] = { id, client };
  }

  function removeCamera(id) {
    delete cameras.value[id];
  }

  function setFilterPattern(pattern) {
    filterPattern.value = pattern;
  }

  function setActiveClientId(mid) {
    activeClientId.value = mid;
  }

  async function upgradeClients() {
    try {
      isUpgrading.value = true;
      await apiService.upgradeClients();
      return { success: true };
    } catch (error) {
      console.error("Failed to upgrade clients:", error);
      return { success: false, error };
    } finally {
      isUpgrading.value = false;
    }
  }

  // Return the store methods and state
  return {
    // State
    clients,
    recentClients,
    fixtures,
    cameras,
    activeClientId,
    filterPattern,
    isUpgrading,

    // Getters
    clientsArray,
    activeRecentClients,
    filteredClients,

    // Actions
    loadInitialClients,
    addClient,
    removeClient,
    addFixture,
    removeFixture,
    addCamera,
    removeCamera,
    setFilterPattern,
    setActiveClientId,
    upgradeClients,
  };
});
