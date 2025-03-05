<template>
  <div
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
import { onMounted } from "vue";
import { monitorService } from "@/services/monitor";
import NavBar from "@/components/NavBar.vue";
import SideBar from "@/components/SideBar.vue";
import TerminalWindow from "@/components/TerminalWindow.vue";
import CameraWindow from "@/components/CameraWindow.vue";
import UploadProgressWidget from "@/components/UploadProgressWidget.vue";
import { useClientStore } from "@/stores/clientStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useUploadProgressStore } from "@/stores/uploadProgressStore";

const clientStore = useClientStore();
const terminalStore = useTerminalStore();
const uploadProgressStore = useUploadProgressStore();

onMounted(async () => {
  // Load initial clients
  await clientStore.loadInitialClients();

  // Set up WebSocket event handlers
  monitorService.on("agent joined", (msg) => {
    const client = JSON.parse(msg);
    clientStore.addClient(client);
  });

  monitorService.on("agent left", (msg) => {
    const client = JSON.parse(msg);
    clientStore.removeClient(client.mid);
  });

  monitorService.on("file download", (sid) => {
    const url = `${window.location.protocol}//${window.location.host}/api/file/download/${sid}`;
    const iframe = document.createElement("iframe");
    iframe.style.display = "none";
    iframe.src = url;
    document.body.appendChild(iframe);
  });
});
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
