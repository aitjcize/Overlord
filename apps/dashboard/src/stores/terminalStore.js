import { defineStore } from "pinia";
import { ref, computed } from "vue";

export const useTerminalStore = defineStore("terminals", () => {
  // State
  const terminals = ref({});
  const highestZIndex = ref(100);
  // Track terminal counts per client
  const clientTerminalCounts = ref({});

  // Getters
  const terminalsArray = computed(() => Object.values(terminals.value));

  // Get all terminals for a specific client
  const getClientTerminals = (clientMid) => {
    return terminalsArray.value.filter((t) => t.mid === clientMid);
  };

  // Check if a client has multiple terminals
  const hasMultipleTerminals = (clientMid) => {
    return getClientTerminals(clientMid).length > 1;
  };

  // Get sequential ID for a client's terminal
  const getSequentialIdForClient = (clientMid) => {
    if (!clientTerminalCounts.value[clientMid]) {
      clientTerminalCounts.value[clientMid] = 1;
    } else {
      clientTerminalCounts.value[clientMid]++;
    }
    return clientTerminalCounts.value[clientMid];
  };

  // Actions
  function addTerminal(id, client, isMobile = false) {
    if (!client || !client.mid) {
      console.error("Invalid client object:", client);
      return null;
    }

    // Get sequential ID for this client
    const sequentialId = getSequentialIdForClient(client.mid);

    const terminal = {
      id,
      mid: client.mid,
      sessionId: client.mid,
      clientName: client.name || client.mid,
      clientSequentialId: sequentialId,
      isMinimized: false,
      isFocused: true,
      zIndex: getNextZIndex(),
      lastStateChange: Date.now(),
    };

    // Add the terminal to the store
    terminals.value[id] = terminal;

    // Only minimize other terminals on mobile
    if (isMobile) {
      minimizeAllTerminalsExcept(id, false, isMobile);
    }

    // Dispatch event for terminal changes
    setTimeout(() => {
      window.dispatchEvent(
        new CustomEvent("terminal-change", {
          detail: {
            type: "add",
            terminal,
            clientMid: client.mid,
          },
        }),
      );
    }, 0);

    return terminal;
  }

  function removeTerminal(id) {
    if (terminals.value[id]) {
      // Get the client mid before removing
      const clientMid = terminals.value[id].mid;

      // Remove the terminal
      delete terminals.value[id];

      // If all terminals for this client are removed, reset the counter
      const remainingTerminals = getClientTerminals(clientMid);
      if (remainingTerminals.length === 0) {
        clientTerminalCounts.value[clientMid] = 0;
      }

      // Dispatch event for terminal changes
      setTimeout(() => {
        window.dispatchEvent(
          new CustomEvent("terminal-change", {
            detail: {
              type: "remove",
              terminalId: id,
              clientMid,
            },
          }),
        );
      }, 0);
    }
  }

  function minimizeAllTerminalsExcept(
    activeId,
    shouldMinimizeActive = false,
    isMobile = false,
  ) {
    // In desktop mode, we only manage focus, not minimization
    if (!isMobile) {
      Object.values(terminals.value).forEach((terminal) => {
        const isActive = terminal.id === activeId;
        terminal.isFocused = isActive;
        if (isActive) {
          terminal.zIndex = getNextZIndex();
        }
        terminal.lastStateChange = Date.now();
      });
    } else {
      // Mobile behavior - minimize all except active
      Object.values(terminals.value).forEach((terminal) => {
        const isActive = terminal.id === activeId;
        const shouldMinimize = isActive ? shouldMinimizeActive : true;

        terminal.isMinimized = shouldMinimize;
        terminal.isFocused = !shouldMinimize && isActive;
        terminal.lastStateChange = Date.now();
      });
    }

    // Dispatch a custom event to notify components
    setTimeout(() => {
      const terminalsArray = Object.values(terminals.value);
      window.dispatchEvent(
        new CustomEvent("terminal-state-changed", {
          detail: { terminals: terminalsArray },
        }),
      );
    }, 0);
  }

  function maximizeTerminal(id) {
    Object.values(terminals.value).forEach((terminal) => {
      const shouldMinimize = terminal.id !== id;

      terminal.isMinimized = shouldMinimize;
      terminal.isFocused = !shouldMinimize;
      terminal.lastStateChange = Date.now();
    });
  }

  function focusTerminal(id) {
    // Get the current highest z-index from all terminals
    const maxZIndex = Math.max(
      highestZIndex.value,
      ...Object.values(terminals.value).map((t) => t.zIndex || 0),
    );

    // Set new highest z-index
    const newZIndex = maxZIndex + 1;
    highestZIndex.value = newZIndex;

    // Update the terminal
    if (terminals.value[id]) {
      terminals.value[id].zIndex = newZIndex;
      terminals.value[id].isFocused = true;
      terminals.value[id].lastStateChange = Date.now();

      // Update focus state for other terminals
      Object.values(terminals.value).forEach((terminal) => {
        if (terminal.id !== id) {
          terminal.isFocused = false;
        }
      });
    }
  }

  function getNextZIndex() {
    return highestZIndex.value + 1;
  }

  function updateTerminalProperty(id, propertyName, value) {
    if (!terminals.value[id]) {
      console.warn(
        `Cannot update property "${propertyName}" on terminal "${id}": terminal not found`,
      );
      return false;
    }

    // Update the property
    terminals.value[id][propertyName] = value;

    // Mark the terminal as updated
    terminals.value[id].lastStateChange = Date.now();

    // Dispatch an event for the terminal update if needed
    // Only do this for important property changes that affect UI
    const importantProperties = ["isMinimized", "isFocused", "zIndex", "sid"];
    if (importantProperties.includes(propertyName)) {
      setTimeout(() => {
        window.dispatchEvent(
          new CustomEvent("terminal-property-updated", {
            detail: {
              terminalId: id,
              property: propertyName,
              value: value,
            },
          }),
        );
      }, 0);
    }

    return true;
  }

  function toggleMinimize(id, isMobile = false) {
    if (!terminals.value[id]) return;

    const terminal = terminals.value[id];

    // Only allow minimizing in mobile mode
    if (!isMobile) {
      // In desktop mode, focus but don't minimize
      focusTerminal(id);
      return;
    }

    // Mobile behavior
    terminal.isMinimized = !terminal.isMinimized;
    terminal.lastStateChange = Date.now();

    // If we're maximizing, focus this terminal and minimize others
    if (!terminal.isMinimized) {
      terminal.isFocused = true;

      // Minimize all other terminals
      Object.values(terminals.value).forEach((t) => {
        if (t.id !== id) {
          t.isMinimized = true;
          t.isFocused = false;
          t.lastStateChange = Date.now();
        }
      });

      // Dispatch an event to notify components about the state change
      setTimeout(() => {
        window.dispatchEvent(
          new CustomEvent("terminal-restored", {
            detail: { terminalId: id },
          }),
        );
      }, 10);
    }
  }

  // Return the store methods and state
  return {
    terminals,
    highestZIndex,
    terminalsArray,
    clientTerminalCounts,
    addTerminal,
    removeTerminal,
    minimizeAllTerminalsExcept,
    maximizeTerminal,
    focusTerminal,
    toggleMinimize,
    hasMultipleTerminals,
    getClientTerminals,
    updateTerminalProperty,
  };
});
