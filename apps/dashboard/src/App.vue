<template>
  <!-- Show login form if not authenticated -->
  <LoginForm v-if="!authStore.isAuthenticated" />

  <!-- Show dashboard if authenticated -->
  <div
    v-else
    class="min-h-screen bg-gradient-to-br from-slate-950 via-slate-900 to-slate-800"
  >
    <SideBar>
      <NavBar />
      <main class="p-4">
        <router-view></router-view>
        <!-- Floating Windows Container -->
        <div class="floating-windows">
          <!-- Terminal Windows -->
          <TerminalWindow
            v-for="terminal in terminalStore.terminalsArray"
            :key="terminal.id"
            :terminal="terminal"
          />
          <!-- Camera Windows -->
          <CameraWindow
            v-for="camera in Object.values(clientStore.cameras)"
            :key="camera.id"
            :camera="camera"
          />
        </div>
      </main>
    </SideBar>

    <!-- Add Upload Progress Widget -->
    <UploadProgressWidget v-if="uploadProgressStore.records.length > 0" />
  </div>
</template>

<script setup>
import { onMounted, watch, computed } from "vue";
import { monitorService } from "@/services/monitor";
import NavBar from "@/components/NavBar.vue";
import SideBar from "@/components/SideBar.vue";
import TerminalWindow from "@/components/TerminalWindow.vue";
import CameraWindow from "@/components/CameraWindow.vue";
import UploadProgressWidget from "@/components/UploadProgressWidget.vue";
import LoginForm from "@/components/LoginForm.vue";
import { useClientStore } from "@/stores/clientStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useUploadProgressStore } from "@/stores/uploadProgressStore";
import { useAuthStore } from "@/stores/authStore";

const clientStore = useClientStore();
const terminalStore = useTerminalStore();
const uploadProgressStore = useUploadProgressStore();
const authStore = useAuthStore();
const isLoggedIn = computed(() => authStore.isLoggedIn);
const isAdmin = computed(() => authStore.userIsAdmin);

onMounted(async () => {
  // Initialize authentication
  authStore.initAuth();

  // Only load data and set up WebSocket if authenticated
  if (authStore.isAuthenticated) {
    initializeDashboard();
  }
});

// Watch for authentication state changes
watch(
  () => authStore.isAuthenticated,
  (isAuthenticated) => {
    if (isAuthenticated) {
      // Start dashboard when authenticated
      initializeDashboard();
    } else {
      // Stop monitor service when logged out
      monitorService.stop();
    }
  },
);

// Function to initialize dashboard data and WebSocket
const initializeDashboard = async () => {
  try {
    // Load initial clients
    await clientStore.loadInitialClients();

    // Start the monitor service
    monitorService.start();

    // Set up WebSocket event handlers
    monitorService.on("agent joined", (msg) => {
      const client = JSON.parse(msg);
      clientStore.addClient(client);
    });

    monitorService.on("agent left", (msg) => {
      const client = JSON.parse(msg);
      clientStore.removeClient(client.mid);
    });

    monitorService.on("file download", (sid, terminalSid) => {
      // Check if the terminalSid is in our list of active terminals
      if (terminalStore.hasTerminalSid(terminalSid)) {
        const token = localStorage.getItem("token");
        const url = `${window.location.protocol}//${window.location.host}/api/sessions/${sid}/file?token=${token}`;
        const iframe = document.createElement("iframe");
        iframe.style.display = "none";
        iframe.src = url;
        document.body.appendChild(iframe);
      }
    });
  } catch (error) {
    console.error("Error initializing dashboard:", error);

    // Check if the error is due to authentication failure
    if (error.response && error.response.status === 401) {
      console.log("Authentication error during dashboard initialization");
      // The axios interceptor will handle the logout
    }
  }
};
</script>

<style>
body {
  @apply bg-slate-950;
}

::-webkit-scrollbar {
  width: 8px;
  height: 8px;
}

::-webkit-scrollbar-track {
  @apply bg-slate-900;
}

::-webkit-scrollbar-thumb {
  @apply bg-slate-700 rounded-full;
}

::-webkit-scrollbar-thumb:hover {
  @apply bg-slate-600;
}

.floating-windows {
  position: fixed;
  top: 4rem; /* Account for navbar height */
  left: 0;
  right: 0;
  bottom: 0;
  pointer-events: none;
  z-index: 30; /* Below navbar z-index */
}

/* Adjust position when sidebar is open in desktop mode */
@media (min-width: 1024px) {
  .floating-windows {
    left: 20rem; /* Account for sidebar width (80 * 4 = 320px = 20rem) */
  }
}

.floating-windows > * {
  pointer-events: auto;
}
</style>
