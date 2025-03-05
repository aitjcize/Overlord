<template>
  <teleport to="body" :disabled="!shouldTeleport">
    <div
      class="card shadow-lg flex flex-col"
      :class="{
        focused: isFocused,
        dragging: isDragging,
        minimized: isMinimized,
        'minimized-stacked': isMinimized && minimizedPosition > 0,
        'desktop-terminal': isLargeScreen && !isMinimized,
      }"
      :key="terminal.id + '-' + (isMinimized ? 'min' : 'max')"
      :style="{
        position: isMinimized ? 'fixed' : isLargeScreen ? 'fixed' : 'relative',
        width: isLargeScreen ? size.width + 'px' : '100%',
        height: isMinimized
          ? headerHeight + 'px'
          : isLargeScreen
            ? size.height + 'px'
            : hasMinimizedTerminals
              ? `calc(100vh - calc(${headerHeight}px * ${minimizedTerminalCount} + 60px))`
              : 'calc(100vh - 60px)',
        left: isLargeScreen ? position.x + 'px' : '0',
        top: isLargeScreen ? position.y + 'px' : 'auto',
        bottom: isMinimized ? `${minimizedPosition}px` : 'auto',
        'z-index': isMinimized
          ? 1000 - (minimizedPosition || 0)
          : currentZIndex,
        'margin-bottom': !isLargeScreen && !isMinimized ? '0' : '0',
        isolation: isLargeScreen && !isMinimized ? 'isolate' : 'auto',
        transform: isLargeScreen && !isMinimized ? 'none' : 'translateZ(0)',
        opacity: isMinimized && minimizedPosition > 0 ? 0.9 : 1,
      }"
      @mousedown="handleMouseDown"
      :data-terminal-id="terminal.id"
      :data-minimized-position="minimizedPosition"
      ref="terminalEl"
    >
      <div
        class="window-header"
        :class="{
          'cursor-move': isLargeScreen,
          'minimized-header': isMinimized,
        }"
        @mousedown="handleHeaderMouseDown"
      >
        <div class="placeholder"></div>
        <h3 class="window-title" ref="titleElement">
          {{ abbreviatedTitle }}
          <span
            v-if="showSequentialId"
            class="terminal-id"
            ref="terminalIdElement"
            >#{{ terminal.clientSequentialId }}</span
          >
        </h3>
        <div class="flex gap-0">
          <button
            v-if="!isLargeScreen"
            class="btn btn-ghost btn-sm btn-square"
            @click.stop="toggleMinimize"
            :title="isMinimized ? 'Maximize' : 'Minimize'"
            :key="`min-btn-${terminal.id}-${isMinimized ? 'min' : 'max'}-${terminal.lastStateChange || 0}`"
          >
            <template v-if="!isMinimized">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  d="M5 14H15C15.5523 14 16 13.5523 16 13C16 12.4477 15.5523 12 15 12H5C4.44772 12 4 12.4477 4 13C4 13.5523 4.44772 14 5 14Z"
                />
              </svg>
            </template>
            <template v-else>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  d="M10 6.5C10.2652 6.5 10.5196 6.60536 10.7071 6.79289L14.7071 10.7929C15.0976 11.1834 15.0976 11.8166 14.7071 12.2071C14.3166 12.5976 13.6834 12.5976 13.2929 12.2071L10 8.91421L6.70711 12.2071C6.31658 12.5976 5.68342 12.5976 5.29289 12.2071C4.90237 11.8166 4.90237 11.1834 5.29289 10.7929L9.29289 6.79289C9.48043 6.60536 9.73478 6.5 10 6.5Z"
                />
              </svg>
            </template>
          </button>
          <button
            v-if="isLargeScreen"
            class="btn btn-ghost btn-sm btn-square"
            @click.stop="toggleFullscreen"
            title="Toggle Fullscreen"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                d="M3 4C3 3.44772 3.44772 3 4 3H16C16.5523 3 17 3.44772 17 4V16C17 16.5523 16.5523 17 16 17H4C3.44772 17 3 16.5523 3 16V4ZM5 5V15H15V5H5Z"
              />
            </svg>
          </button>
          <button
            class="btn btn-ghost btn-sm btn-square"
            @click="closeTerminal"
            title="Close"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-4 w-4"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                fill-rule="evenodd"
                d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z"
                clip-rule="evenodd"
              />
            </svg>
          </button>
        </div>
      </div>
      <div
        v-show="!isMinimized"
        class="terminal-container"
        ref="terminalContainer"
        :class="{ fullscreen: isFullscreen }"
        :style="{
          display: isMinimized ? 'none !important' : 'flex',
          visibility: isMinimized ? 'hidden' : 'visible',
        }"
      >
        <div
          class="terminal-overlay terminal-drop-overlay"
          :class="{ active: isDropActive }"
          ref="dropOverlay"
        >
          Drop files here to upload
        </div>
      </div>

      <!-- Resize handles (only shown when not minimized) -->
      <template v-if="!isMinimized">
        <div
          class="resize-handle top"
          @mousedown.stop="startResize('top')"
        ></div>
        <div
          class="resize-handle right"
          @mousedown.stop="startResize('right')"
        ></div>
        <div
          class="resize-handle bottom"
          @mousedown.stop="startResize('bottom')"
        ></div>
        <div
          class="resize-handle left"
          @mousedown.stop="startResize('left')"
        ></div>
        <div
          class="resize-handle top-right"
          @mousedown.stop="startResize('top-right')"
        ></div>
        <div
          class="resize-handle bottom-right"
          @mousedown.stop="startResize('bottom-right')"
        ></div>
        <div
          class="resize-handle bottom-left"
          @mousedown.stop="startResize('bottom-left')"
        ></div>
        <div
          class="resize-handle top-left"
          @mousedown.stop="startResize('top-left')"
        ></div>
      </template>
    </div>
  </teleport>
</template>

<script setup>
import { ref, onMounted, onBeforeUnmount, computed, watch, inject } from "vue";
import { Terminal } from "xterm";
import { FitAddon } from "xterm-addon-fit";
import { WebLinksAddon } from "xterm-addon-web-links";
import "xterm/css/xterm.css";
import { nextTick } from "vue";
import { useTerminalStore } from "@/stores/terminalStore";
import { useUploadProgressStore } from "@/stores/uploadProgressStore";

const terminalStore = useTerminalStore();
const uploadProgressStore = useUploadProgressStore();

// Inject the sidebar width from SideBar component
const injectedSidebarWidth = inject("sidebarWidth", ref(320));

const props = defineProps({
  terminal: {
    type: Object,
    required: true,
  },
});

// Add computed property for focus state
const isFocused = computed(() => {
  const focused = props.terminal.isFocused;
  return focused;
});

const isMinimized = computed(() => {
  // Get the minimized state and the last state change timestamp for reactivity
  const minimized = props.terminal.isMinimized;

  // When minimized, handle body class immediately
  if (minimized) {
    document.body.classList.add("has-minimized-terminal");
  } else if (document.body.classList.contains("has-minimized-terminal")) {
    // Check if there are any other minimized terminals before removing the class
    const terminals = terminalStore.terminalsArray;
    const anyMinimized = terminals.some(
      (t) => t.id !== props.terminal.id && t.isMinimized,
    );
    if (!anyMinimized) {
      document.body.classList.remove("has-minimized-terminal");
    }
  }

  return minimized;
});

// Add computed property to determine if we should show the sequential ID
const showSequentialId = computed(() => {
  return terminalStore.hasMultipleTerminals(props.terminal.mid);
});

const terminalContainer = ref(null);
const titleElement = ref(null);
const terminalEl = ref(null);
const isFullscreen = ref(false);
let xterm = null;
let fitAddon = null;
let ws = null;

const position = ref({ x: 0, y: 0 });
const isDragging = ref(false);
const dragStart = ref({ x: 0, y: 0 });

const size = ref({ width: 600, height: 400 });
const isResizing = ref(false);
const resizeStart = ref({ x: 0, y: 0, width: 0, height: 0 });
const resizeDirection = ref(null);

const resizeTimeout = ref(null);

// Use a ref for current z-index to avoid accessing the prop directly
const currentZIndex = ref(props.terminal.zIndex || 0);

const isLargeScreen = ref(window.innerWidth >= 1024);

// Add a ref for custom title set by terminal control sequences
const customTitle = ref("");

// Add a computed property for header height based on screen size
const headerHeight = computed(() => (!isLargeScreen.value ? 36 : 48));

// Add computed properties to check if there are minimized terminals
const hasMinimizedTerminals = computed(() => {
  const terminals = terminalStore.terminalsArray;
  // Only count other terminals, not this one
  return terminals.some((t) => t.id !== props.terminal.id && t.isMinimized);
});

const minimizedTerminalCount = computed(() => {
  const terminals = terminalStore.terminalsArray;
  // Count the number of minimized terminals
  return terminals.filter((t) => t.isMinimized).length;
});

// Create a composable for managing body classes
const useBodyClass = () => {
  // Keep a reference count of how many components need each class
  const bodyClassesRefCount = ref(new Map());

  // Add class to body with reference counting
  const addClass = (className) => {
    const count = bodyClassesRefCount.value.get(className) || 0;
    bodyClassesRefCount.value.set(className, count + 1);

    if (count === 0) {
      document.body.classList.add(className);
    }
  };

  // Remove class with reference counting
  const removeClass = (className) => {
    const count = bodyClassesRefCount.value.get(className) || 0;

    if (count <= 1) {
      bodyClassesRefCount.value.delete(className);
      document.body.classList.remove(className);
    } else {
      bodyClassesRefCount.value.set(className, count - 1);
    }
  };

  // Clean up on component unmount
  onBeforeUnmount(() => {
    // Clean up any classes this component added
    bodyClassesRefCount.value.forEach((count, className) => {
      if (count <= 1) {
        document.body.classList.remove(className);
      }
    });
  });

  return {
    addClass,
    removeClass,
  };
};

// Use the body class composable
const { addClass, removeClass } = useBodyClass();

// Watch for minimized state changes and update body classes
watch(
  isMinimized,
  (minimized) => {
    if (minimized) {
      addClass("has-minimized-terminal");
    } else {
      removeClass("has-minimized-terminal");
    }
  },
  { immediate: true },
);

// Create a more reactive approach to managing CSS variables
const useCssVariables = () => {
  const setCssVariable = (name, value) => {
    document.documentElement.style.setProperty(name, value);
  };

  return { setCssVariable };
};

const { setCssVariable } = useCssVariables();

// Watch the minimized terminal count and update CSS variables
watch(
  minimizedTerminalCount,
  (count) => {
    setCssVariable("--minimized-terminal-count", Math.max(1, count));
  },
  { immediate: true },
);

// Update the minimizedPosition computed property to use this function
const minimizedPosition = computed(() => {
  if (!isMinimized.value) return null;

  // Get all minimized terminals and their indices
  const terminals = terminalStore.terminalsArray;
  const minimizedTerminals = terminals.filter((t) => t.isMinimized);

  // If no minimized terminals, return 0
  if (minimizedTerminals.length === 0) {
    return 0;
  }

  // Sort minimized terminals consistently by creation time (oldest first)
  minimizedTerminals.sort((a, b) => {
    // First try to sort by ID (which often contains a timestamp component)
    return a.id.localeCompare(b.id);
  });

  // Find this terminal's index
  const terminalIndex = minimizedTerminals.findIndex(
    (t) => t.id === props.terminal.id,
  );

  // If this terminal wasn't found in the minimized list, return 0
  if (terminalIndex === -1) {
    return 0;
  }

  // Calculate position from the bottom using the headerHeight computed property
  // Important: we stack from the bottom up, so the first terminal (index 0) is at the bottom
  const position = terminalIndex * headerHeight.value;

  // Update the CSS variable for minimized count
  setCssVariable("--minimized-terminal-count", minimizedTerminals.length);

  return position;
});

const sendTerminalResize = (cols, rows) => {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    return;
  }

  // Send standard ANSI escape sequence for window size
  // Format: ESC [ t {rows} ; {cols} t
  const sequence = `\x1b[8;${rows};${cols}t`;
  ws.send(sequence);
};

// Consolidate terminal management functions
const terminalUtils = {
  fit: () => {
    if (!fitAddon || !xterm || !terminalContainer.value || isMinimized.value)
      return false;

    try {
      fitAddon.fit();
      const { cols, rows } = xterm;
      sendTerminalResize(cols, rows);
      return true;
    } catch (error) {
      console.error("Error fitting terminal:", error);
      return false;
    }
  },

  redraw: () => {
    if (!xterm) return false;
    terminalUtils.fit();
    xterm.write(""); // Force a redraw
    return true;
  },

  reattach: async () => {
    if (!terminalContainer.value || !xterm) return false;

    try {
      // Instead of disposing, just reattach the terminal
      if (xterm.element && xterm.element.parentNode) {
        xterm.element.parentNode.removeChild(xterm.element);
      }

      // Reopen terminal in container
      xterm.open(terminalContainer.value);

      // Reconnect WebSocket if needed
      reconnectWebSocket();

      // Resize and redraw
      terminalUtils.fit();

      return true;
    } catch (error) {
      console.error("Error re-attaching terminal:", error);
      return false;
    }
  },

  checkAndRestore: () => {
    nextTick(() => {
      setTimeout(() => {
        if (
          terminalContainer.value &&
          !terminalContainer.value.querySelector(".xterm")
        ) {
          terminalUtils.reattach();
        } else {
          terminalUtils.redraw();
        }
      }, 50);
    });
  },
};

const handleTerminalResize = () => {
  terminalUtils.fit();
};

const handleFullscreenChange = () => {
  isFullscreen.value = document.fullscreenElement === terminalContainer.value;
  handleTerminalResize();
};

// Update the handleTerminalRestored function to use the utility
const handleTerminalRestored = (event) => {
  if (event.detail.terminalId === props.terminal.id) {
    terminalUtils.checkAndRestore();
  }
};

// Update the toggleMinimize function to use the utility
const toggleMinimize = (e) => {
  if (!isLargeScreen.value) {
    e.stopPropagation();
    const wasMinimized = props.terminal.isMinimized;
    terminalStore.toggleMinimize(props.terminal.id, !isLargeScreen.value);

    if (wasMinimized) {
      terminalUtils.checkAndRestore();
    }
  }
};

// Update screen size check
const updateScreenSize = () => {
  const wasLargeScreen = isLargeScreen.value;
  isLargeScreen.value = window.innerWidth >= 1024;

  // Initialize position for desktop mode
  if (
    isLargeScreen.value &&
    (!position.value.x || !position.value.y || !wasLargeScreen)
  ) {
    // Stagger the position of each terminal in desktop mode
    const terminalCount = terminalStore.terminalsArray.length;
    const offsetBase = 30;
    const offsetX = (terminalCount % 5) * offsetBase;
    const offsetY = (terminalCount % 5) * offsetBase;

    // Get sidebar width from injected ref
    const sidebarWidth = injectedSidebarWidth.value;

    // Calculate right boundary to ensure terminal stays within the viewable area
    const viewportWidth = window.innerWidth;
    const terminalWidth = size.value.width || 600; // Default or current width
    const maxX = viewportWidth - terminalWidth - 20; // Stay 20px from right edge

    // Calculate position, ensuring it's right of sidebar but still in viewport
    const desiredX = Math.max(sidebarWidth + 20, sidebarWidth + 20 + offsetX);
    const safeX = Math.min(desiredX, maxX);

    position.value = {
      x: safeX,
      y: Math.max(50, 50 + offsetY),
    };
  }
};

// Check if terminal view should be minimized on mobile
const checkMobileMinimizedView = () => {
  if (!isLargeScreen.value) {
    // Force the terminal to be minimized on mobile if it's not focused
    if (!props.terminal.isFocused) {
      props.terminal.isMinimized = true;
      props.terminal.lastStateChange = Date.now();
    }
  }
};

// Store event handler references for cleanup
let terminalStateChangedHandler = null;
let resizeHandler = null;
let terminalChangeHandler = null;
let unwatch = null;

// Add terminal configuration constants
const TERMINAL_THEME = {
  background: "#000000",
  foreground: "#e5e7eb",
  cursor: "#e5e7eb",
  selection: "#374151",
  black: "#000000",
  red: "#ef4444",
  green: "#22c55e",
  yellow: "#f59e0b",
  blue: "#3b82f6",
  magenta: "#8b5cf6",
  cyan: "#06b6d4",
  white: "#f3f4f6",
  brightBlack: "#4b5563",
  brightRed: "#f87171",
  brightGreen: "#4ade80",
  brightYellow: "#fbbf24",
  brightBlue: "#60a5fa",
  brightMagenta: "#a78bfa",
  brightCyan: "#22d3ee",
  brightWhite: "#ffffff",
};

const TERMINAL_OPTIONS = {
  cursorBlink: true,
  fontSize: 14,
  fontFamily: 'Menlo, Monaco, "Courier New", monospace',
  convertEol: true,
  scrollback: 10000,
  allowProposedApi: true,
  theme: TERMINAL_THEME,
  rendererType: "canvas",
  disableStdin: false,
};

// Add refs for the drop overlay
const dropOverlay = ref(null);

// Add local refs for terminal state after the existing props
const terminalSid = ref(props.terminal.sid);
const isDropActive = ref(false);

// Create a computed property for the terminalId to use in uploads
const activeTerminalId = computed(() => {
  return terminalSid.value || props.terminal.mid;
});

// Update the uploadRequestPath computed to use the local state
const uploadRequestPath = computed(() => {
  return `/api/agent/upload/${props.terminal.mid}`;
});

// After terminal import statements, add this composable
/**
 * Composable for handling file drag and drop
 */
const useFileDragDrop = (options) => {
  const { terminalId, uploadPath, onUpload } = options;

  const isActive = ref(false);

  const handleDragEnter = (event) => {
    event.preventDefault();
    event.stopPropagation();
    isActive.value = true;
  };

  const handleDragOver = (event) => {
    event.preventDefault();
    event.stopPropagation();
  };

  const handleDragLeave = (event) => {
    event.preventDefault();
    event.stopPropagation();
    isActive.value = false;
  };

  const handleDrop = (event) => {
    event.preventDefault();
    event.stopPropagation();
    const files = event.dataTransfer.files;

    for (let i = 0; i < files.length; i++) {
      if (onUpload) {
        onUpload(files[i], terminalId.value);
      }
    }
    isActive.value = false;
  };

  return {
    isActive,
    handleDragEnter,
    handleDragOver,
    handleDragLeave,
    handleDrop,
  };
};

// Replace the bindDragAndDropEvents function with a more composable-driven approach
const bindDragAndDropEvents = () => {
  // Only proceed if we have access to the DOM elements
  if (!terminalContainer.value || !dropOverlay.value) return;

  // Get the actual terminal DOM element
  const terminalDom = terminalContainer.value.querySelector(".terminal");
  const overlay = dropOverlay.value;

  if (!terminalDom) return;

  // Use our composable for drag and drop functionality
  const {
    isActive,
    handleDragEnter,
    handleDragOver,
    handleDragLeave,
    handleDrop,
  } = useFileDragDrop({
    terminalId: activeTerminalId,
    uploadPath: uploadRequestPath,
    onUpload: (file, termId) => {
      // Check if we're using fallback
      if (!terminalSid.value) {
        console.warn(
          "Terminal SID not available, using MID as fallback for upload. This may cause issues if SID is required.",
        );
      }

      // Perform the upload
      uploadProgressStore.upload(
        uploadRequestPath.value,
        file,
        undefined,
        termId,
      );
    },
  });

  // Bind the isActive from the composable to our component state
  watch(isActive, (active) => {
    isDropActive.value = active;
  });

  // Attach events using addEventListener
  terminalDom.addEventListener("dragenter", handleDragEnter);
  overlay.addEventListener("dragenter", handleDragEnter);
  overlay.addEventListener("dragover", handleDragOver);
  overlay.addEventListener("dragleave", handleDragLeave);
  overlay.addEventListener("drop", handleDrop);

  // Clean up event listeners on component unmount
  onBeforeUnmount(() => {
    terminalDom.removeEventListener("dragenter", handleDragEnter);
    overlay.removeEventListener("dragenter", handleDragEnter);
    overlay.removeEventListener("dragover", handleDragOver);
    overlay.removeEventListener("dragleave", handleDragLeave);
    overlay.removeEventListener("drop", handleDrop);
  });
};

// Setup terminal
const setupTerminal = () => {
  try {
    xterm = new Terminal(TERMINAL_OPTIONS);

    // Add addons
    fitAddon = new FitAddon();
    xterm.loadAddon(fitAddon);
    xterm.loadAddon(new WebLinksAddon());

    // Register title change handler
    xterm.onTitleChange((title) => {
      customTitle.value = title;
    });

    // Open terminal in container
    xterm.open(terminalContainer.value);

    // Set up drag and drop events after the terminal is created
    nextTick(() => {
      bindDragAndDropEvents();
    });

    // Fit terminal to container and get initial size
    try {
      fitAddon.fit();
      const { cols, rows } = xterm;
      sendTerminalResize(cols, rows);
    } catch (error) {
      console.error("Error during terminal fit:", error);
    }

    // Reconnect WebSocket if needed
    reconnectWebSocket();

    return true;
  } catch (error) {
    console.error("Error setting up terminal:", error);
    return false;
  }
};

// Add a function to setup WebSocket connection
const setupWebSocket = () => {
  if (!props.terminal.mid) {
    console.error("Terminal missing mid:", props.terminal);
    if (xterm) {
      xterm.write("\x1b[1;31mError: Invalid terminal configuration\x1b[0m\r\n");
    }
    return null;
  }

  // Create a new WebSocket connection
  const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
  const wsUrl = `${protocol}//${window.location.host}/api/agent/tty/${props.terminal.mid}`;
  const newWs = new WebSocket(wsUrl);

  newWs.onopen = () => {
    if (fitAddon && xterm) {
      try {
        fitAddon.fit();
        const { cols, rows } = xterm;
        sendTerminalResize(cols, rows);
      } catch (error) {
        console.error("Error during WebSocket open terminal fit:", error);
      }
    }
  };

  newWs.onmessage = (event) => {
    if (event.data instanceof Blob) {
      // Handle binary data
      const reader = new FileReader();
      reader.onload = () => {
        const data = new Uint8Array(reader.result);
        // Convert binary data to string
        const text = new TextDecoder().decode(data);
        xterm.write(text);
      };
      reader.readAsArrayBuffer(event.data);
    } else {
      // Handle text data
      try {
        // Debug: Log the raw message to see its structure
        console.log("WebSocket message received:", event.data);

        const data = JSON.parse(event.data);
        if (data.type === "sid") {
          // Session ID received - use data.data as the SID (matches React implementation)
          // Store in our local ref instead of mutating the prop directly
          terminalSid.value = data.data;
          console.log("Terminal session ID received:", data.data);
        } else {
          // If JSON but not a SID message, write to terminal
          xterm.write(event.data);
        }
      } catch (e) {
        // If not JSON, treat as regular terminal output
        xterm.write(event.data);
      }
    }
  };

  newWs.onerror = (error) => {
    console.error("WebSocket error:", error);
    if (xterm) {
      xterm.write(
        "\r\n\x1b[1;31mConnection error occurred. Terminal may not respond.\x1b[0m\r\n",
      );
    }
  };

  newWs.onclose = () => {
    // WebSocket closed
    if (xterm) {
      // Send red text to terminal indicating disconnection
      xterm.write(
        "\r\n\x1b[1;31mConnection closed. Terminal disconnected.\x1b[0m\r\n",
      );
    }
  };

  // Handle terminal input
  if (xterm) {
    xterm.onData((data) => {
      if (newWs && newWs.readyState === WebSocket.OPEN) {
        newWs.send(data);
      }
    });
  }

  return newWs;
};

// Add a function to reconnect the WebSocket if needed
const reconnectWebSocket = () => {
  if (!ws || ws.readyState !== WebSocket.OPEN) {
    // Close existing connection if any
    if (ws) {
      try {
        ws.close();
      } catch (e) {
        console.error("Error closing existing WebSocket:", e);
      }
    }

    // Setup new connection
    ws = setupWebSocket();
    return ws !== null;
  }

  return false;
};

// Add a function to ensure the terminal is attached to the right parent
const ensureProperDOMParent = () => {
  if (isLargeScreen.value && !isMinimized.value && terminalEl.value) {
    // In desktop mode, move to document.body for proper stacking
    nextTick(() => {
      if (terminalEl.value && terminalEl.value.parentNode !== document.body) {
        document.body.appendChild(terminalEl.value);
      }
    });
  }
};

onMounted(() => {
  try {
    updateScreenSize(); // Initial check

    // Call our function to ensure proper DOM parent
    ensureProperDOMParent();

    // Set up resize handler that includes terminal resizing
    resizeHandler = () => {
      // Update screen size
      updateScreenSize();

      // If switching to mobile view, check if we need to minimize
      const wasLargeScreen = isLargeScreen.value;
      isLargeScreen.value = window.innerWidth >= 1024;

      if (wasLargeScreen && !isLargeScreen.value) {
        checkMobileMinimizedView();
      }

      // Also handle terminal resize
      handleTerminalResize();
    };
    window.addEventListener("resize", resizeHandler);

    // Add listener for terminal state changes
    terminalStateChangedHandler = (event) => {
      const { terminals } = event.detail;
      const thisTerminal = terminals.find((t) => t.id === props.terminal.id);

      if (thisTerminal) {
        // Update our local terminal's state to match the event data
        if (props.terminal.isMinimized !== thisTerminal.isMinimized) {
          props.terminal.isMinimized = thisTerminal.isMinimized;
          props.terminal.lastStateChange = Date.now();
        }
      }

      // Update minimized count
      setCssVariable(
        "--minimized-terminal-count",
        minimizedTerminalCount.value,
      );
    };
    window.addEventListener(
      "terminal-state-changed",
      terminalStateChangedHandler,
    );

    // Setup terminal
    setupTerminal();

    // Add fullscreenchange event listener
    document.addEventListener("fullscreenchange", handleFullscreenChange);

    // Update current z-index
    currentZIndex.value = props.terminal.zIndex;

    // Minimize all other windows when this one is created (mobile only)
    if (!isLargeScreen.value) {
      terminalStore.minimizeAllTerminalsExcept(
        props.terminal.id,
        false,
        !isLargeScreen.value,
      );
    } else {
      // In desktop mode, just focus this terminal without minimizing others
      terminalStore.focusTerminal(props.terminal.id);
    }

    // Check mobile view
    checkMobileMinimizedView();

    // Initialize minimized count
    setCssVariable("--minimized-terminal-count", minimizedTerminalCount.value);

    // Set up a watcher to handle terminal resize when maximized
    watch(
      () => props.terminal.isMinimized,
      () => {
        // Only proceed if mobile view or transitioning out of minimized state
        if (
          !isLargeScreen.value ||
          (isLargeScreen.value && !props.terminal.isMinimized)
        ) {
          // Force terminal resize after maximizing
          if (!props.terminal.isMinimized && fitAddon) {
            terminalUtils.checkAndRestore();
          }
        }

        // Update body class (only for mobile)
        if (!isLargeScreen.value) {
          if (props.terminal.isMinimized) {
            addClass("has-minimized-terminal");
          } else {
            // Check if all terminals are maximized before removing the class
            const anyMinimized = terminalStore.terminalsArray.some(
              (t) => t.id !== props.terminal.id && t.isMinimized,
            );

            if (!anyMinimized) {
              removeClass("has-minimized-terminal");
            }
          }
        }

        // Update minimized count
        setCssVariable(
          "--minimized-terminal-count",
          minimizedTerminalCount.value,
        );
      },
    );

    // Event listener for terminal restored after minimize
    window.addEventListener("terminal-restored", handleTerminalRestored);

    // Add event listener for terminal changes that might affect the display of IDs
    terminalChangeHandler = () => {
      // Force reactivity update for the showSequentialId computed property
      nextTick(() => {
        terminalStore.hasMultipleTerminals(props.terminal.mid);
      });
    };
    window.addEventListener("terminal-change", terminalChangeHandler);

    // Set up a watcher for DOM positioning
    unwatch = watch([isMinimized, isLargeScreen], () => {
      ensureProperDOMParent();
    });

    // Add handler for terminal state changes
    const originalHandler = terminalStateChangedHandler;
    terminalStateChangedHandler = (event) => {
      // Call the original handler first
      if (originalHandler) originalHandler(event);

      // Update the minimized count
      setCssVariable(
        "--minimized-terminal-count",
        minimizedTerminalCount.value,
      );
    };

    // Add additional handler for terminal changes
    const originalChangeHandler = terminalChangeHandler;
    terminalChangeHandler = (event) => {
      // Call the original handler first
      if (originalChangeHandler) originalChangeHandler(event);

      // Update the minimized count
      setCssVariable(
        "--minimized-terminal-count",
        minimizedTerminalCount.value,
      );
    };

    // Add a more robust resize handler that ensures minimized positions are updated
    const originalResizeHandler = resizeHandler;
    resizeHandler = () => {
      // Call the original resize handler
      if (originalResizeHandler) originalResizeHandler();

      // Force a complete refresh of minimized terminal positions
      if (isMinimized.value) {
        // Trigger all terminals to recalculate their positions
        setCssVariable(
          "--minimized-terminal-count",
          minimizedTerminalCount.value,
        );

        // Force Vue to update by toggling a property
        setTimeout(() => {
          // This will force Vue to reevaluate the minimizedPosition computed property
          props.terminal.lastStateChange = Date.now();

          // Also trigger an update for all other minimized terminals
          const minimizedTerminals = terminalStore.terminalsArray.filter(
            (t) => t.isMinimized,
          );
          minimizedTerminals.forEach((t) => {
            if (t.id !== props.terminal.id) {
              t.lastStateChange = Date.now();
            }
          });
        }, 10);
      }
    };

    // Add ResizeObserver to update title when terminal size changes
    if (titleElement.value && window.ResizeObserver) {
      const resizeObserver = new ResizeObserver(() => {
        updateTitle();
      });

      resizeObserver.observe(titleElement.value);

      // Clean up observer
      onBeforeUnmount(() => {
        resizeObserver.disconnect();
      });
    }

    // Initial update
    setCssVariable("--minimized-terminal-count", minimizedTerminalCount.value);

    // Add watchers for repositioning when injected sidebar width changes
    watch(injectedSidebarWidth, () => {
      if (isLargeScreen.value && !isMinimized.value) {
        // Only update position if the terminal is already positioned away from default
        if (position.value.x > 0 && position.value.y > 0) {
          // Calculate new position based on updated sidebar width
          const viewportWidth = window.innerWidth;
          const terminalWidth = size.value.width || 600;
          const sidebarWidth = injectedSidebarWidth.value;
          const maxX = viewportWidth - terminalWidth - 20;

          // Ensure terminal stays right of the sidebar
          if (position.value.x < sidebarWidth + 20) {
            position.value.x = Math.min(sidebarWidth + 20, maxX);
          }
        }
      }
    });
  } catch (error) {
    console.error("Error in terminal mounting:", error);
  }
});

onBeforeUnmount(() => {
  // Cleanup event listeners
  window.removeEventListener("resize", resizeHandler);
  window.removeEventListener("terminal-restored", handleTerminalRestored);

  if (terminalChangeHandler) {
    window.removeEventListener("terminal-change", terminalChangeHandler);
  }

  if (terminalStateChangedHandler) {
    window.removeEventListener(
      "terminal-state-changed",
      terminalStateChangedHandler,
    );
  }

  // Clean up watcher
  if (unwatch) {
    unwatch();
  }

  // Clean up fullscreen change listener
  document.removeEventListener("fullscreenchange", handleFullscreenChange);

  // Close WebSocket connection
  if (ws) {
    try {
      ws.close();
    } catch (error) {
      console.error("Error closing WebSocket:", error);
    }
  }

  // Dispose of xterm instance
  if (xterm) {
    try {
      xterm.dispose();
    } catch (error) {
      console.error("Error disposing xterm:", error);
    }
  }
});

const toggleFullscreen = async () => {
  try {
    if (!document.fullscreenElement) {
      await terminalContainer.value.requestFullscreen();
    } else {
      await document.exitFullscreen();
    }
  } catch (error) {
    console.error("Error toggling fullscreen:", error);
  }
};

const closeTerminal = () => {
  terminalStore.removeTerminal(props.terminal.id);
};

const handleMouseDown = (e) => {
  // Don't handle clicks on buttons
  if (e.target.closest("button")) {
    return;
  }

  // Always focus the window first
  focus();
};

const startDragging = (e) => {
  // Prevent default to avoid text selection
  e.preventDefault();

  // Focus the window first
  focus();

  isDragging.value = true;
  dragStart.value = {
    x: e.clientX - position.value.x,
    y: e.clientY - position.value.y,
  };

  // Add dragging class to body to prevent text selection
  document.body.classList.add("dragging");

  window.addEventListener("mousemove", handleDrag);
  window.addEventListener("mouseup", stopDragging);
};

const handleDrag = (e) => {
  if (!isDragging.value) return;

  requestAnimationFrame(() => {
    position.value = {
      x: e.clientX - dragStart.value.x,
      y: e.clientY - dragStart.value.y,
    };
  });
};

const stopDragging = () => {
  if (!isDragging.value) return;

  isDragging.value = false;
  // Remove dragging class from body
  document.body.classList.remove("dragging");

  window.removeEventListener("mousemove", handleDrag);
  window.removeEventListener("mouseup", stopDragging);
};

const startResize = (direction) => {
  // Focus the window first
  focus();

  isResizing.value = true;
  resizeDirection.value = direction;
  resizeStart.value = {
    x: event.clientX,
    y: event.clientY,
    width: size.value.width,
    height: size.value.height,
    left: position.value.x,
    top: position.value.y,
  };

  window.addEventListener("mousemove", handleResize);
  window.addEventListener("mouseup", stopResize);
  event.preventDefault();
};

const handleResize = (event) => {
  if (!isResizing.value) return;

  requestAnimationFrame(() => {
    const deltaX = event.clientX - resizeStart.value.x;
    const deltaY = event.clientY - resizeStart.value.y;

    const newSize = { ...size.value };
    const newPosition = { ...position.value };

    let newWidth, newHeight;

    switch (resizeDirection.value) {
      case "right":
        newSize.width = Math.max(300, resizeStart.value.width + deltaX);
        break;
      case "left":
        newWidth = Math.max(300, resizeStart.value.width - deltaX);
        if (newWidth !== size.value.width) {
          newPosition.x =
            resizeStart.value.left + (resizeStart.value.width - newWidth);
          newSize.width = newWidth;
        }
        break;
      case "bottom":
        newSize.height = Math.max(200, resizeStart.value.height + deltaY);
        break;
      case "top":
        newHeight = Math.max(200, resizeStart.value.height - deltaY);
        if (newHeight !== size.value.height) {
          newPosition.y =
            resizeStart.value.top + (resizeStart.value.height - newHeight);
          newSize.height = newHeight;
        }
        break;
      case "top-right":
        newSize.width = Math.max(300, resizeStart.value.width + deltaX);
        newHeight = Math.max(200, resizeStart.value.height - deltaY);
        if (newHeight !== size.value.height) {
          newPosition.y =
            resizeStart.value.top + (resizeStart.value.height - newHeight);
          newSize.height = newHeight;
        }
        break;
      case "bottom-right":
        newSize.width = Math.max(300, resizeStart.value.width + deltaX);
        newSize.height = Math.max(200, resizeStart.value.height + deltaY);
        break;
      case "bottom-left":
        newWidth = Math.max(300, resizeStart.value.width - deltaX);
        if (newWidth !== size.value.width) {
          newPosition.x =
            resizeStart.value.left + (resizeStart.value.width - newWidth);
          newSize.width = newWidth;
        }
        newSize.height = Math.max(200, resizeStart.value.height + deltaY);
        break;
      case "top-left":
        newWidth = Math.max(300, resizeStart.value.width - deltaX);
        if (newWidth !== size.value.width) {
          newPosition.x =
            resizeStart.value.left + (resizeStart.value.width - newWidth);
          newSize.width = newWidth;
        }
        newHeight = Math.max(200, resizeStart.value.height - deltaY);
        if (newHeight !== size.value.height) {
          newPosition.y =
            resizeStart.value.top + (resizeStart.value.height - newHeight);
          newSize.height = newHeight;
        }
        break;
    }

    size.value = newSize;
    position.value = newPosition;

    // Debounce the terminal resize
    if (resizeTimeout.value) clearTimeout(resizeTimeout.value);
    resizeTimeout.value = setTimeout(handleTerminalResize, 16);
  });
};

const stopResize = () => {
  isResizing.value = false;
  resizeDirection.value = null;

  window.removeEventListener("mousemove", handleResize);
  window.removeEventListener("mouseup", stopResize);

  // Final resize
  handleTerminalResize();
};

const focus = () => {
  try {
    // Use the terminal store to focus this terminal
    terminalStore.focusTerminal(props.terminal.id);

    // Update our local z-index ref
    currentZIndex.value = props.terminal.zIndex;
  } catch (error) {
    console.error("Error in focus handler:", error);
    // Fallback if there's an error
    currentZIndex.value = terminalStore.highestZIndex + 1;
  }
};

const handleHeaderMouseDown = (e) => {
  // Don't handle clicks on buttons
  if (e.target.closest("button")) {
    return;
  }

  if (isLargeScreen.value) {
    startDragging(e);
  }

  // Always focus the window
  focus();
};

// Add watcher for terminal state changes
watch(
  () => props.terminal.isMinimized,
  () => {
    // Force update of minimized positions
    nextTick(() => {
      setCssVariable(
        "--minimized-terminal-count",
        minimizedTerminalCount.value,
      );
    });

    // Also ensure proper DOM parent
    ensureProperDOMParent();
  },
);

// Add a watcher to respond to changes in minimized terminal count
watch(minimizedTerminalCount, () => {
  // Force redraw of terminal if needed
  if (!isMinimized.value && !isLargeScreen.value) {
    nextTick(() => {
      if (fitAddon && xterm) {
        setTimeout(() => {
          handleTerminalResize();
        }, 50);
      }
    });
  }
});

// Extract title-related measurements into separate computed properties
const titleWidth = computed(() => {
  if (!titleElement.value) return 0;
  return titleElement.value.clientWidth || 0;
});

// Function to get font metrics without direct DOM manipulation
const useFontMetrics = () => {
  const fontMetricsCache = ref({
    avgCharWidth: 0,
    ellipsisWidth: 0,
    fontStyle: "",
  });

  // Calculate font metrics in a more performant way
  const calculateFontMetrics = (element) => {
    if (!element) return;

    try {
      // Get the computed style for accurate font measurement
      const computedStyle = window.getComputedStyle(element);
      const fontStyle = `${computedStyle.fontWeight} ${computedStyle.fontSize} ${computedStyle.fontFamily}`;

      // Only recalculate if the font style has changed
      if (fontStyle !== fontMetricsCache.value.fontStyle) {
        const canvas = document.createElement("canvas");
        const context = canvas.getContext("2d");
        context.font = fontStyle;

        // Calculate metrics
        const sampleText =
          "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
        const sampleWidth = context.measureText(sampleText).width;
        const avgCharWidth = sampleWidth / sampleText.length;
        const ellipsisWidth = context.measureText("...").width;

        // Update cache
        fontMetricsCache.value = {
          avgCharWidth,
          ellipsisWidth,
          fontStyle,
        };
      }
    } catch (error) {
      console.error("Error calculating font metrics:", error);
    }
  };

  return {
    fontMetricsCache,
    calculateFontMetrics,
  };
};

// Get font metrics functions
const { fontMetricsCache, calculateFontMetrics } = useFontMetrics();

// Terminal ID width calculation
const terminalIdElement = ref(null);

// Improve the reactive terminal ID width to handle conditional rendering
const terminalIdWidth = computed(() => {
  // First check if we should even show the ID
  if (!showSequentialId.value) return 0;

  // Then check if the element is rendered
  if (!terminalIdElement.value) return 0;

  // Finally return the width with padding
  return terminalIdElement.value.offsetWidth + 4;
});

// Calculate max chars for title truncation
const maxTitleChars = computed(() => {
  const defaultMaxChars = isLargeScreen.value ? 60 : 30;

  // If title element doesn't exist, return default
  if (!titleElement.value) return defaultMaxChars;

  try {
    // Ensure we have font metrics
    calculateFontMetrics(titleElement.value);

    // Get the available width
    const availableWidth = titleWidth.value;
    if (!availableWidth) return defaultMaxChars;

    // Calculate effective width with safety margin
    const effectiveWidth =
      availableWidth -
      terminalIdWidth.value -
      fontMetricsCache.value.ellipsisWidth -
      20;

    // Determine how many characters would fit
    const maxChars = Math.max(
      15,
      Math.floor(effectiveWidth / fontMetricsCache.value.avgCharWidth),
    );

    return maxChars;
  } catch (error) {
    console.error("Error calculating max chars:", error);
    return defaultMaxChars;
  }
});

// Clean up abbreviatedTitle computed using the other computed properties
const abbreviatedTitle = computed(() => {
  const title = customTitle.value || props.terminal.clientName;

  // Early return if no title or very short title
  if (!title || title.length < 10) {
    return title;
  }

  // Get calculated max chars
  const maxChars = maxTitleChars.value;

  // Truncate if necessary
  if (title.length > maxChars) {
    return title.substring(0, maxChars - 3) + "...";
  }

  return title;
});

// More reactive title update approach
const updateTitle = () => {
  // Just ensure the calculation is triggered again
  calculateFontMetrics(titleElement.value);
  // Since customTitle and other dependencies will trigger a recomputation
  // of abbreviatedTitle, we don't need the hack with clearing and setting
};

// After terminalSid declaration
// Add a watcher to sync sid changes with the parent component if needed
watch(terminalSid, (newSid) => {
  // This is where you would emit an event to the parent if needed
  // emit('update:sid', newSid);

  console.log(`Terminal SID updated: ${newSid}`);

  // If we need to update the parent terminal object, we can do so via the store
  if (props.terminal && newSid) {
    // This is a direct mutation but handled through the store instead of props
    terminalStore.updateTerminalProperty(props.terminal.id, "sid", newSid);
  }
});
</script>

<style lang="scss" scoped>
.card {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border: 1px solid rgba(51, 65, 85, 0.5);
  border-radius: 0.5rem;
  box-shadow:
    0 4px 6px -1px rgba(0, 0, 0, 0.3),
    0 2px 4px -1px rgba(0, 0, 0, 0.2);
  will-change: transform;
  backface-visibility: hidden;
  transform: translateZ(0);
  background-color: rgba(15, 23, 42, 0.75);
  opacity: 0.95;
  transition: all 0.2s ease;
  min-width: 300px;
  min-height: 200px;
  resize: none;

  &.desktop-terminal {
    position: fixed !important;
    transform: none !important; /* Remove transform to avoid creating a stacking context */
    isolation: isolate !important; /* Isolate this element from the rest of the page */
    contain: none !important; /* Ensure element isn't contained */
    pointer-events: auto !important; /* Ensure clicks are captured */
  }

  &.focused {
    background-color: rgba(15, 23, 42, 0.98);
    border-color: rgba(51, 65, 85, 0.7);
    box-shadow:
      0 0 0 1px rgba(16, 185, 129, 0.4),
      0 0 0 3px rgba(16, 185, 129, 0.4),
      0 10px 15px -3px rgba(0, 0, 0, 0.4),
      0 4px 6px -2px rgba(0, 0, 0, 0.3),
      0 0 30px rgba(16, 185, 129, 0.5),
      0 0 45px rgba(16, 185, 129, 0.4),
      0 0 60px rgba(16, 185, 129, 0.3);
    opacity: 1;
  }

  &:hover:not(.focused) {
    opacity: 0.98;
    box-shadow:
      0 4px 6px -1px rgba(0, 0, 0, 0.3),
      0 2px 4px -1px rgba(0, 0, 0, 0.2),
      0 0 25px rgba(16, 185, 129, 0.25);
  }

  &.dragging {
    transition: none;
  }

  @media (max-width: 1023px) {
    position: relative !important;
    margin: 0;
    width: 100% !important;
    border-radius: 0.5rem;
    border: 1px solid rgba(51, 65, 85, 0.5);
    z-index: 10 !important;
    transform: none !important;
    max-width: none !important;
    min-width: 0 !important;

    &.minimized {
      position: fixed !important;
      left: 0 !important;
      right: 0 !important;
      width: 100% !important;
      height: 36px !important;
      min-height: 36px !important;
      margin: 0 !important;
      padding: 0 !important;
      border-bottom-left-radius: 0 !important;
      border-bottom-right-radius: 0 !important;
      border-top: 2px solid rgba(16, 185, 129, 0.5) !important;
      border-radius: 0 !important;
      box-shadow: 0 -1px 3px rgba(0, 0, 0, 0.1) !important;
      background-color: rgba(15, 23, 42, 0.85) !important;
      display: block !important;
      visibility: visible !important;
      pointer-events: auto !important;

      &::after {
        display: none;
      }

      .window-header {
        border-bottom: none !important;
        padding: 0.25rem 0.5rem !important;
      }

      .terminal-container {
        display: none !important;
        height: 0 !important;
        visibility: hidden !important;
      }

      .resize-handle {
        display: none !important;
      }

      &.minimized-stacked {
        border-top: 2px solid rgba(59, 130, 246, 0.5) !important;
        box-shadow: 0 -2px 4px rgba(0, 0, 0, 0.15) !important;

        .window-header {
          background-color: rgba(15, 23, 42, 0.95) !important;
        }

        &:hover {
          opacity: 1 !important;
          box-shadow: 0 -3px 6px rgba(0, 0, 0, 0.2) !important;
        }
      }
    }

    &.focused {
      box-shadow:
        0 0 0 1px rgba(16, 185, 129, 0.25),
        0 0 0 2px rgba(16, 185, 129, 0.2),
        0 5px 10px -3px rgba(0, 0, 0, 0.3),
        0 2px 4px -2px rgba(0, 0, 0, 0.2),
        0 0 15px rgba(16, 185, 129, 0.25),
        0 0 20px rgba(16, 185, 129, 0.15);
    }
  }
}

.window-header {
  padding: 0.25rem;
  background-color: rgba(15, 23, 42, 0.95);
  color: #94a3b8;
  display: grid;
  grid-template-columns: minmax(2rem, auto) minmax(0, 1fr) minmax(auto, auto);
  align-items: center;
  border-bottom: 1px solid rgba(51, 65, 85, 0.5);
  user-select: none;
  border-top-left-radius: 0.5rem;
  border-top-right-radius: 0.5rem;
  backdrop-filter: blur(12px);

  @media (max-width: 1023px) {
    border-radius: 0.5rem 0.5rem 0 0;
    padding: 0.5rem;
  }

  .focused & {
    color: #e2e8f0;
  }

  &.minimized-header {
    transition: background-color 0.2s ease;

    &:hover {
      // Removed background-color change on hover
    }
  }
}

.window-title {
  margin: 0;
  font-size: 0.875rem;
  font-weight: 500;
  color: inherit;
  text-align: center;
  grid-column: 2;
  justify-self: center;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-width: 0;
  width: 100%;

  .terminal-id {
    display: inline-block;
    font-size: 0.75rem;
    font-weight: 600;
    color: rgba(16, 185, 129, 0.9);
    margin-left: 0.25rem;
    border-radius: 0.25rem;
    padding: 0 0.25rem;
    background-color: rgba(16, 185, 129, 0.1);
  }
}

.placeholder {
  grid-column: 1;
  min-width: 1rem;
  width: auto;
  padding-left: 0.5rem;
}

.flex.gap-0 {
  grid-column: 3;
  justify-content: flex-end;
  display: flex;
  min-width: max-content;
  padding-right: 0.25rem;
}

.terminal-container {
  flex: 1;
  background-color: #000000;
  overflow: hidden;
  border-bottom-left-radius: 0.5rem;
  border-bottom-right-radius: 0.5rem;

  @media (max-width: 1023px) {
    border-radius: 0 0 0.5rem 0.5rem;
    width: 100%;
    margin: 0;
  }
}

.terminal-container.fullscreen {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 1000;
  padding: 0;
  border-radius: 0;
}

.cursor-move {
  cursor: move;
}

:deep(.xterm) {
  width: 100%;
  height: 100%;

  .xterm-viewport {
    background-color: #000000 !important;
  }
}

.resize-handle {
  position: absolute;
  background: transparent;
  z-index: 10;

  &.top,
  &.bottom {
    height: 6px;
    width: calc(100% - 12px);
    left: 6px;
    cursor: ns-resize;
  }

  &.left,
  &.right {
    width: 6px;
    height: calc(100% - 12px);
    top: 6px;
    cursor: ew-resize;
  }

  &.top {
    top: -3px;
  }

  &.right {
    right: -3px;
  }

  &.bottom {
    bottom: -3px;
  }

  &.left {
    left: -3px;
  }

  &.top-right,
  &.bottom-right,
  &.bottom-left,
  &.top-left {
    width: 12px;
    height: 12px;
  }

  &.top-right {
    top: -3px;
    right: -3px;
    cursor: nesw-resize;
  }

  &.bottom-right {
    bottom: -3px;
    right: -3px;
    cursor: nwse-resize;
  }

  &.bottom-left {
    bottom: -3px;
    left: -3px;
    cursor: nesw-resize;
  }

  &.top-left {
    top: -3px;
    left: -3px;
    cursor: nwse-resize;
  }

  &:hover {
    background: rgba(16, 185, 129, 0.2);
  }
}

:deep(body.dragging) {
  user-select: none;
  cursor: move !important;
}

.btn-ghost {
  color: inherit;
  opacity: 0.7;

  &:hover {
    opacity: 1;
    color: #10b981;
  }
}

@media (max-width: 1023px) {
  :deep(body) {
    overflow-x: hidden !important;
    overflow-y: auto !important;
    width: 100%;
    position: relative;
    padding: 0;
  }

  .resize-handle {
    display: none;
  }

  .window-header {
    border-radius: 0.5rem 0.5rem 0 0;
    padding: 0.25rem 0.5rem;
    height: 36px;
    display: flex;
    align-items: center;
  }

  .card {
    &.minimized {
      position: fixed !important;
      left: 0 !important;
      right: 0 !important;
      width: 100% !important;
      height: 36px !important;
      min-height: 36px !important;
      margin: 0 !important;
      padding: 0 !important;
      border-bottom-left-radius: 0 !important;
      border-bottom-right-radius: 0 !important;
      border-top: 2px solid rgba(16, 185, 129, 0.5) !important;
      border-radius: 0 !important;
      box-shadow: 0 -1px 3px rgba(0, 0, 0, 0.1) !important;
      background-color: rgba(15, 23, 42, 0.85) !important;
      display: block !important;
      visibility: visible !important;
      pointer-events: auto !important;

      &::after {
        display: none;
      }

      .window-header {
        border-bottom: none !important;
        padding: 0.25rem 0.5rem !important;
      }

      .terminal-container {
        display: none !important;
        height: 0 !important;
        visibility: hidden !important;
      }

      .resize-handle {
        display: none !important;
      }

      &.minimized-stacked {
        border-top: 2px solid rgba(59, 130, 246, 0.5) !important;
        box-shadow: 0 -2px 4px rgba(0, 0, 0, 0.15) !important;

        .window-header {
          background-color: rgba(15, 23, 42, 0.95) !important;
        }

        &:hover {
          opacity: 1 !important;
          box-shadow: 0 -3px 6px rgba(0, 0, 0, 0.2) !important;
        }
      }
    }

    &.focused {
      box-shadow:
        0 0 0 1px rgba(16, 185, 129, 0.25),
        0 0 0 2px rgba(16, 185, 129, 0.2),
        0 5px 10px -3px rgba(0, 0, 0, 0.3),
        0 2px 4px -2px rgba(0, 0, 0, 0.2),
        0 0 15px rgba(16, 185, 129, 0.25),
        0 0 20px rgba(16, 185, 129, 0.15);
    }
  }

  // Add padding to body based on the number of minimized terminals
  // This ensures that content is never hidden behind the stacked terminals
  body.has-minimized-terminal {
    padding-bottom: calc(
      36px * var(--minimized-terminal-count, 1) + 5px
    ) !important;
    transition: padding-bottom 0.2s ease;
  }
}

/* Only apply padding for minimized terminals on mobile */
@media (min-width: 1024px) {
  body.has-minimized-terminal {
    padding-bottom: 0 !important;
  }
}

.terminal-overlay {
  display: none;
  width: 100%;
  height: 100%;
  position: absolute;
  top: 0;
  left: 0;
  text-align: center;
  font-size: 2em;
  font-weight: bold;
  color: #333;
  z-index: 100;
  align-items: center;
  justify-content: center;
}

.terminal-drop-overlay {
  background-color: rgba(15, 23, 42, 0.85);
  border: 4px dashed rgba(16, 185, 129, 0.6);
  border-radius: 0.5rem;
  color: #e5e7eb;
  text-shadow: 0 0 10px rgba(16, 185, 129, 0.5);
  backdrop-filter: blur(4px);
  box-shadow:
    inset 0 0 20px rgba(16, 185, 129, 0.2),
    0 0 15px rgba(16, 185, 129, 0.3);
}

/* When overlay is shown, use flex display */
.terminal-overlay.active {
  display: flex;
}
</style>
