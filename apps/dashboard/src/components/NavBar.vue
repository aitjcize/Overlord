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

    <div class="navbar-end gap-2"></div>
  </div>
</template>

<script setup>
// No imports needed
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
