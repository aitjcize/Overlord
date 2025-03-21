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
            <a
              @click="openPasswordModal"
              class="text-slate-300 hover:text-slate-100"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5 mr-2"
                viewBox="0 0 20 20"
                fill="currentColor"
              >
                <path
                  fill-rule="evenodd"
                  d="M5 9V7a5 5 0 0110 0v2a2 2 0 012 2v5a2 2 0 01-2 2H5a2 2 0 01-2-2v-5a2 2 0 012-2zm8-2v2H7V7a3 3 0 016 0z"
                  clip-rule="evenodd"
                />
              </svg>
              Change Password
            </a>
          </li>
          <li>
            <a @click="handleLogout" class="text-red-400 hover:text-red-300">
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-5 w-5 mr-2"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
              >
                <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path>
                <polyline points="16 17 21 12 16 7"></polyline>
                <line x1="21" y1="12" x2="9" y2="12"></line>
              </svg>
              Logout
            </a>
          </li>
        </ul>
      </div>
    </div>
  </div>

  <!-- Change Password Modal -->
  <div
    v-if="passwordModal.isOpen"
    class="fixed inset-0 z-50 overflow-auto bg-black bg-opacity-50 flex justify-center items-center"
  >
    <div class="bg-slate-800 p-6 rounded-lg shadow-lg max-w-md w-full">
      <h3 class="text-lg font-medium text-white mb-4">Change Your Password</h3>
      <form @submit.prevent="changePassword" class="space-y-4">
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-1"
            >Current Password</label
          >
          <input
            v-model="passwordModal.currentPassword"
            type="password"
            required
            class="w-full px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="Enter current password"
          />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-1"
            >New Password</label
          >
          <input
            v-model="passwordModal.newPassword"
            type="password"
            required
            class="w-full px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="Enter new password"
          />
        </div>
        <div>
          <label class="block text-sm font-medium text-gray-300 mb-1"
            >Confirm New Password</label
          >
          <input
            v-model="passwordModal.confirmPassword"
            type="password"
            required
            class="w-full px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            placeholder="Confirm new password"
          />
        </div>
        <div v-if="passwordModal.error" class="text-red-400 text-sm">
          {{ passwordModal.error }}
        </div>
        <div class="flex justify-end space-x-3">
          <button
            type="button"
            @click="closePasswordModal"
            class="px-4 py-2 bg-slate-600 hover:bg-slate-700 text-white rounded-md shadow-sm"
          >
            Cancel
          </button>
          <button
            type="submit"
            :disabled="passwordModal.loading"
            class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md shadow-sm"
          >
            <span v-if="passwordModal.loading">Processing...</span>
            <span v-else>Change Password</span>
          </button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup>
import { useAuthStore } from "@/stores/authStore";
import { useUserStore } from "@/stores/userStore";
import { reactive } from "vue";

const authStore = useAuthStore();
const userStore = useUserStore();

// Password change modal state
const passwordModal = reactive({
  isOpen: false,
  currentPassword: "",
  newPassword: "",
  confirmPassword: "",
  error: null,
  loading: false,
});

// Open the password change modal
const openPasswordModal = () => {
  passwordModal.isOpen = true;
  passwordModal.currentPassword = "";
  passwordModal.newPassword = "";
  passwordModal.confirmPassword = "";
  passwordModal.error = null;
};

// Close the password change modal
const closePasswordModal = () => {
  passwordModal.isOpen = false;
};

// Change the user's password
const changePassword = async () => {
  // Reset error
  passwordModal.error = null;

  // Validate passwords match
  if (passwordModal.newPassword !== passwordModal.confirmPassword) {
    passwordModal.error = "New passwords don't match";
    return;
  }

  // Validate password complexity (optional)
  if (passwordModal.newPassword.length < 6) {
    passwordModal.error = "Password must be at least 6 characters";
    return;
  }

  try {
    passwordModal.loading = true;

    // Use the updated userStore method that now takes current password
    await userStore.updateUserPassword(
      authStore.user.username,
      passwordModal.newPassword,
      passwordModal.currentPassword,
    );

    // Close the modal on success
    closePasswordModal();

    // Show success notification (you could use a notification system here)
    alert("Password changed successfully");
  } catch (error) {
    console.error("Error changing password:", error);
    passwordModal.error = error.message || "Failed to change password";
  } finally {
    passwordModal.loading = false;
  }
};

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
