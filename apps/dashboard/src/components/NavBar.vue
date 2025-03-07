<template>
  <div
    class="navbar bg-gradient-to-r from-slate-900 to-slate-800 border-b border-slate-700/50 px-4"
  >
    <div class="flex-1">
      <label for="sidebar-drawer" class="btn btn-ghost btn-square lg:hidden">
        <svg
          xmlns="http://www.w3.org/2000/svg"
          class="h-5 w-5"
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 6h16M4 12h16M4 18h7"
          />
        </svg>
      </label>
      <h1 class="text-xl font-semibold">
        <span class="text-emerald-500/80 mr-2">OVERLORD</span>
        <span class="text-slate-400">Dashboard</span>
      </h1>
    </div>

    <div class="navbar-center hidden lg:flex">
      <!-- Removed Dashboard link -->
    </div>

    <div class="navbar-end gap-2">
      <!-- User info and logout button -->
      <div class="dropdown dropdown-end">
        <label tabindex="0" class="btn btn-ghost btn-circle avatar">
          <div
            class="w-10 rounded-full bg-emerald-600 flex items-center justify-center"
          >
            <span
              class="text-white text-lg font-bold flex items-center justify-center h-full w-full"
            >
              {{
                authStore.user && authStore.user.username
                  ? authStore.user.username.charAt(0).toUpperCase()
                  : "U"
              }}
            </span>
          </div>
        </label>
        <ul
          tabindex="0"
          class="mt-3 p-2 shadow menu menu-compact dropdown-content bg-slate-800 rounded-box w-52 border border-slate-700"
        >
          <li>
            <div class="text-slate-300 pointer-events-none">
              <span>Signed in as</span>
              <span class="font-bold">{{
                authStore.user?.username || "User"
              }}</span>
            </div>
          </li>
          <li>
            <a @click="handleLogout" class="text-red-400 hover:text-red-300"
              >Logout</a
            >
          </li>
        </ul>
      </div>
    </div>
  </div>
</template>

<script setup>
import { useAuthStore } from "@/stores/authStore";

const authStore = useAuthStore();

const handleLogout = () => {
  authStore.logout();
  // Reload the page to ensure clean state
  window.location.reload();
};
</script>

<style lang="scss" scoped>
.navbar {
  padding: 0.5rem 1rem;
  box-shadow:
    0 4px 6px -1px rgba(0, 0, 0, 0.3),
    0 2px 4px -1px rgba(0, 0, 0, 0.2);
  backdrop-filter: blur(12px);
}

.btn-ghost {
  transition: all 0.2s ease;

  &:hover {
    transform: translateY(-1px);
  }
}
</style>
