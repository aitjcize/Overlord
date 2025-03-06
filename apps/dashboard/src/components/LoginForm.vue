<template>
  <div class="flex items-center justify-center min-h-screen px-4">
    <div
      class="w-full max-w-md p-8 space-y-6 bg-slate-800 rounded-lg shadow-2xl"
    >
      <div class="text-center">
        <h1 class="text-3xl font-bold text-white">Overlord</h1>
        <p class="mt-2 text-gray-400">Sign in to access the dashboard</p>
      </div>

      <form @submit.prevent="handleLogin" class="space-y-4">
        <div>
          <label for="username" class="block text-sm font-medium text-gray-300"
            >Username</label
          >
          <input
            id="username"
            v-model="username"
            type="text"
            required
            class="w-full px-3 py-2 mt-1 text-white bg-slate-700 border border-slate-600 rounded-md focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:border-transparent"
            :disabled="authStore.loading"
          />
        </div>

        <div>
          <label for="password" class="block text-sm font-medium text-gray-300"
            >Password</label
          >
          <input
            id="password"
            v-model="password"
            type="password"
            required
            class="w-full px-3 py-2 mt-1 text-white bg-slate-700 border border-slate-600 rounded-md focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:border-transparent"
            :disabled="authStore.loading"
          />
        </div>

        <div
          v-if="authStore.error"
          class="p-3 text-sm text-red-300 bg-red-900/50 border border-red-800 rounded-md"
        >
          {{ authStore.error }}
        </div>

        <button
          type="submit"
          class="w-full px-4 py-2 text-sm font-medium text-white bg-emerald-600 rounded-md hover:bg-emerald-700 focus:outline-none focus:ring-2 focus:ring-emerald-500 focus:ring-offset-2 focus:ring-offset-slate-800 disabled:opacity-50 disabled:cursor-not-allowed"
          :disabled="authStore.loading"
        >
          <span v-if="authStore.loading">
            <svg
              class="inline w-4 h-4 mr-2 animate-spin"
              xmlns="http://www.w3.org/2000/svg"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle
                class="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                stroke-width="4"
              ></circle>
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              ></path>
            </svg>
            Signing in...
          </span>
          <span v-else>Sign in</span>
        </button>
      </form>
    </div>
  </div>
</template>

<script setup>
import { ref } from "vue";
import { useAuthStore } from "@/stores/authStore";

const authStore = useAuthStore();
const username = ref("");
const password = ref("");

const handleLogin = async () => {
  if (username.value && password.value) {
    await authStore.login(username.value, password.value);
  }
};
</script>
