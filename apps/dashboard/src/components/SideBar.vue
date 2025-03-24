<template>
  <div class="drawer lg:drawer-open">
    <input
      id="sidebar-drawer"
      type="checkbox"
      class="drawer-toggle"
      ref="drawerToggle"
    />

    <div class="drawer-content flex flex-col">
      <!-- Page content here -->
      <slot></slot>
    </div>

    <div class="drawer-side z-40">
      <label
        for="sidebar-drawer"
        aria-label="close sidebar"
        class="drawer-overlay"
      ></label>
      <div
        ref="sidebarRef"
        class="bg-gradient-to-b from-slate-900 to-slate-800 w-80 min-h-full p-4 flex flex-col gap-6 border-r border-slate-700/50"
      >
        <!-- Navigation Links -->
        <div class="mb-2">
          <router-link
            to="/"
            class="flex items-center gap-2 p-2 rounded-md text-slate-300 hover:bg-slate-700/50 hover:text-white transition-all"
            :class="{ 'bg-slate-700/70 text-white': $route.path === '/' }"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              class="h-5 w-5"
              viewBox="0 0 20 20"
              fill="currentColor"
            >
              <path
                d="M10.707 2.293a1 1 0 00-1.414 0l-7 7a1 1 0 001.414 1.414L4 10.414V17a1 1 0 001 1h2a1 1 0 001-1v-2a1 1 0 011-1h2a1 1 0 011 1v2a1 1 0 001 1h2a1 1 0 001-1v-6.586l.293.293a1 1 0 001.414-1.414l-7-7z"
              />
            </svg>
            <span>Dashboard</span>
          </router-link>
        </div>

        <!-- Admin Navigation - Only visible to admin users -->
        <div v-if="authStore.userIsAdmin" class="space-y-4">
          <div class="flex items-center justify-between">
            <h3 class="font-semibold text-sm uppercase text-emerald-500/80">
              Administration
            </h3>
            <button
              @click="toggleAdminSection"
              class="text-emerald-500/80 hover:text-emerald-400 p-1 rounded-full"
              :title="
                authStore.preferences.hideAdminButtons
                  ? 'Show admin buttons'
                  : 'Hide admin buttons'
              "
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                viewBox="0 0 20 20"
                fill="currentColor"
                v-if="!authStore.preferences.hideAdminButtons"
              >
                <path
                  fill-rule="evenodd"
                  d="M5 10a1 1 0 011-1h8a1 1 0 110 2H6a1 1 0 01-1-1z"
                  clip-rule="evenodd"
                />
              </svg>
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                viewBox="0 0 20 20"
                fill="currentColor"
                v-else
              >
                <path
                  fill-rule="evenodd"
                  d="M10 5a1 1 0 011 1v3h3a1 1 0 110 2h-3v3a1 1 0 11-2 0v-3H6a1 1 0 110-2h3V6a1 1 0 011-1z"
                  clip-rule="evenodd"
                />
              </svg>
            </button>
          </div>
          <div class="space-y-1" v-if="authStore.showAdminButtons">
            <router-link
              to="/admin/users"
              class="flex items-center gap-2 p-2 rounded-md text-slate-300 hover:bg-slate-700/50 hover:text-white transition-all"
              :class="{
                'bg-slate-700/70 text-white': $route.path === '/admin/users',
              }"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  d="M9 6a3 3 0 11-6 0 3 3 0 016 0zM17 6a3 3 0 11-6 0 3 3 0 016 0zM12.93 17c.046-.327.07-.66.07-1a6.97 6.97 0 00-1.5-4.33A5 5 0 0119 16v1h-6.07zM6 11a5 5 0 015 5v1H1v-1a5 5 0 015-5z"
                />
              </svg>
              <span>User Management</span>
            </router-link>
            <router-link
              to="/admin/groups"
              class="flex items-center gap-2 p-2 rounded-md text-slate-300 hover:bg-slate-700/50 hover:text-white transition-all"
              :class="{
                'bg-slate-700/70 text-white': $route.path === '/admin/groups',
              }"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  d="M13 6a3 3 0 11-6 0 3 3 0 016 0zM18 8a2 2 0 11-4 0 2 2 0 014 0zM14 15a4 4 0 00-8 0v3h8v-3zM6 8a2 2 0 11-4 0 2 2 0 014 0zM16 18v-3a5.972 5.972 0 00-.75-2.906A3.005 3.005 0 0119 15v3h-3zM4.75 12.094A5.973 5.973 0 004 15v3H1v-3a3 3 0 013.75-2.906z"
                />
              </svg>
              <span>Group Management</span>
            </router-link>

            <!-- Upgrade All Clients Button -->
            <button
              @click="triggerUpgrade"
              class="flex items-center gap-2 p-2 rounded-md text-slate-300 hover:bg-slate-700/50 hover:text-white transition-all w-full"
              :disabled="clientStore.isUpgrading"
              :class="{
                'opacity-70': clientStore.isUpgrading,
              }"
            >
              <span
                v-if="clientStore.isUpgrading"
                class="loading loading-spinner loading-sm"
              ></span>
              <svg
                v-else
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fill-rule="evenodd"
                  d="M10 17a1 1 0 001-1v-5.586l3.293 3.293a1 1 0 001.414-1.414l-5-5a1 1 0 00-1.414 0l-5 5a1 1 0 101.414 1.414L9 10.414V16a1 1 0 001 1z"
                  clip-rule="evenodd"
                />
              </svg>
              <span>{{ upgradeButtonText }}</span>
            </button>
          </div>
        </div>

        <!-- Connected Clients -->
        <div class="space-y-4 overflow-y-auto">
          <h3 class="font-semibold text-sm uppercase text-emerald-500/80">
            Connected Clients
          </h3>

          <!-- Search Filter - Moved under Connected Clients title -->
          <div class="form-control mb-2">
            <input
              type="text"
              placeholder="Search clients..."
              class="input bg-slate-800/50 border-slate-700 w-full text-slate-300 placeholder:text-slate-500 focus:border-emerald-500/50 focus:ring-1 focus:ring-emerald-500/50"
              v-model="clientStore.filterPattern"
            />
          </div>

          <div class="space-y-2">
            <div
              v-for="client in clientStore.filteredClients"
              :key="client.mid"
              class="card bg-slate-800/50 shadow-lg hover:shadow-emerald-500/5 transition-all cursor-pointer hover:bg-slate-700/50 border border-slate-700/50"
              :class="{ 'border-l-4 !border-l-emerald-500': client.isActive }"
              @click="selectClient(client)"
            >
              <div class="card-body p-4">
                <div class="flex justify-between items-center">
                  <h4 class="text-sm font-medium text-slate-300">
                    {{ client.name || client.mid }}
                  </h4>
                  <div class="flex gap-2">
                    <button
                      class="btn btn-sm btn-ghost btn-square text-slate-400 hover:text-emerald-400 hover:bg-slate-700/70"
                      @click.stop="openTerminal(client)"
                      title="Open Terminal"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        viewBox="0 0 20 20"
                        fill="currentColor"
                      >
                        <path
                          fill-rule="evenodd"
                          d="M2 5a2 2 0 012-2h12a2 2 0 012 2v10a2 2 0 01-2 2H4a2 2 0 01-2-2V5zm3.293 1.293a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 01-1.414-1.414L7.586 10 5.293 7.707a1 1 0 010-1.414zM11 12a1 1 0 100 2h3a1 1 0 100-2h-3z"
                          clip-rule="evenodd"
                        />
                      </svg>
                    </button>
                    <button
                      v-if="client.hasCamera"
                      class="btn btn-sm btn-ghost btn-square text-slate-400 hover:text-emerald-400 hover:bg-slate-700/70"
                      @click.stop="openCamera(client)"
                      title="Open Camera"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        viewBox="0 0 20 20"
                        fill="currentColor"
                      >
                        <path
                          d="M2 6a2 2 0 012-2h6a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6zm12.553 1.106A1 1 0 0014 8v4a1 1 0 00.553.894l2 1A1 1 0 0018 13V7a1 1 0 00-1.447-.894l-2 1z"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Recent Clients -->
        <div
          class="space-y-4"
          v-if="clientStore.activeRecentClients.length > 0"
        >
          <h3 class="font-semibold text-sm uppercase text-emerald-500/80">
            Recent Clients
          </h3>
          <div class="space-y-2">
            <div
              v-for="client in clientStore.activeRecentClients"
              :key="client.mid"
              class="card bg-slate-800/30 shadow-lg hover:shadow-emerald-500/5 transition-all cursor-pointer hover:bg-slate-700/30 border border-slate-700/30"
              @click="selectClient(client)"
            >
              <div class="card-body p-4">
                <div class="flex justify-between items-center">
                  <h4 class="text-sm font-medium text-slate-400">
                    {{ client.name || client.mid }}
                  </h4>
                  <div class="flex gap-2">
                    <button
                      class="btn btn-sm btn-ghost btn-square text-slate-500 hover:text-emerald-400 hover:bg-slate-700/70"
                      @click.stop="openTerminal(client)"
                      title="Open Terminal"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        viewBox="0 0 20 20"
                        fill="currentColor"
                      >
                        <path
                          fill-rule="evenodd"
                          d="M2 5a2 2 0 012-2h12a2 2 0 012 2v10a2 2 0 01-2 2H4a2 2 0 01-2-2V5zm3.293 1.293a1 1 0 011.414 0l3 3a1 1 0 010 1.414l-3 3a1 1 0 01-1.414-1.414L7.586 10 5.293 7.707a1 1 0 010-1.414zM11 12a1 1 0 100 2h3a1 1 0 100-2h-3z"
                          clip-rule="evenodd"
                        />
                      </svg>
                    </button>
                    <button
                      v-if="client.hasCamera"
                      class="btn btn-sm btn-ghost btn-square text-slate-500 hover:text-emerald-400 hover:bg-slate-700/70"
                      @click.stop="openCamera(client)"
                      title="Open Camera"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        viewBox="0 0 20 20"
                        fill="currentColor"
                      >
                        <path
                          d="M2 6a2 2 0 012-2h6a2 2 0 012 2v8a2 2 0 01-2 2H4a2 2 0 01-2-2V6zm12.553 1.106A1 1 0 0014 8v4a1 1 0 00.553.894l2 1A1 1 0 0018 13V7a1 1 0 00-1.447-.894l-2 1z"
                        />
                      </svg>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Spacer to fill the remaining space -->
        <div class="flex-grow"></div>

        <!-- Note: Upgrade button has been moved to the Admin section -->
      </div>
    </div>
  </div>
</template>

<script setup>
import { useClientStore } from "@/stores/clientStore";
import { useTerminalStore } from "@/stores/terminalStore";
import { useAuthStore } from "@/stores/authStore";
import { ref, onMounted, provide, onBeforeUnmount, watch } from "vue";

const clientStore = useClientStore();
const terminalStore = useTerminalStore();
const authStore = useAuthStore();
const upgradeStatus = ref("idle"); // 'idle', 'success', 'error'
const upgradeButtonText = ref("Upgrade All Clients");

// Reset upgrade status after a delay
const resetUpgradeStatus = () => {
  setTimeout(() => {
    upgradeStatus.value = "idle";
    upgradeButtonText.value = "Upgrade All Clients";
  }, 3000);
};

// Trigger client upgrade
const triggerUpgrade = async () => {
  upgradeButtonText.value = "Upgrading...";
  const result = await clientStore.upgradeClients();

  if (result.success) {
    upgradeStatus.value = "success";
    upgradeButtonText.value = "Upgrade Initiated!";
  } else {
    upgradeStatus.value = "error";
    upgradeButtonText.value = "Upgrade Failed";
  }

  resetUpgradeStatus();
};

const selectClient = (client) => {
  clientStore.setActiveClientId(client.mid);
  // Close drawer on mobile after selection using the ref instead of direct DOM access
  if (window.innerWidth < 1024 && drawerToggle.value) {
    drawerToggle.value.checked = false;
  }
};

const openTerminal = (client) => {
  const terminalId = `${client.mid}-${Date.now()}`;
  const isMobile = window.innerWidth < 1024;
  terminalStore.addTerminal(terminalId, client, isMobile);
  // Close drawer on mobile after opening terminal using the ref
  if (isMobile && drawerToggle.value) {
    drawerToggle.value.checked = false;
  }
};

const openCamera = (client) => {
  const cameraId = `camera-${client.mid}-${Date.now()}`;
  clientStore.addCamera(cameraId, client);
  // Close drawer on mobile after opening camera using the ref
  if (window.innerWidth < 1024 && drawerToggle.value) {
    drawerToggle.value.checked = false;
  }
};

// Create a composable for responsive detection
const useResponsive = () => {
  const isLargeScreen = ref(window.innerWidth >= 1024);

  const checkScreenSize = () => {
    isLargeScreen.value = window.innerWidth >= 1024;
    return isLargeScreen.value;
  };

  onMounted(() => {
    checkScreenSize();
    window.addEventListener("resize", checkScreenSize);
  });

  onBeforeUnmount(() => {
    window.removeEventListener("resize", checkScreenSize);
  });

  return {
    isLargeScreen,
    checkScreenSize,
  };
};

// Use the responsive composable
const { isLargeScreen } = useResponsive();

// Reference to sidebar for width measurement
const sidebarRef = ref(null);
const sidebarWidth = ref(320); // Default width

// Reference to the drawer toggle checkbox
const drawerToggle = ref(null);

// Update function to consider drawer state
const updateSidebarWidth = () => {
  if (sidebarRef.value) {
    // If on mobile and drawer is closed, sidebar width should be 0
    if (
      !isLargeScreen.value &&
      drawerToggle.value &&
      !drawerToggle.value.checked
    ) {
      sidebarWidth.value = 0;
    } else {
      sidebarWidth.value = sidebarRef.value.offsetWidth;
    }
  }
};

// Watch drawer toggle changes
watch(
  () => drawerToggle.value?.checked,
  () => {
    // Small delay to allow for transition
    setTimeout(updateSidebarWidth, 300);
  },
  { immediate: true },
);

// Also watch screen size changes
watch(
  isLargeScreen,
  () => {
    updateSidebarWidth();
  },
  { immediate: true },
);

// Expose sidebar width via provide/inject pattern
provide("sidebarWidth", sidebarWidth);

// Update width on mount
onMounted(() => {
  updateSidebarWidth();
});

// Function to toggle admin section visibility
const toggleAdminSection = () => {
  authStore.toggleAdminButtons();
};
</script>

<style lang="scss" scoped>
.drawer-side {
  .menu {
    padding: 1rem;

    a {
      color: #475569;
      opacity: 0.9;
      transition: all 0.2s ease;

      &:hover {
        opacity: 1;
        background-color: rgba(59, 130, 246, 0.08);
      }

      &.active {
        background-color: rgba(59, 130, 246, 0.12);
        color: #2563eb;
        opacity: 1;
      }
    }
  }
}
</style>
