<template>
  <div class="p-6">
    <h1 class="text-2xl font-bold text-white mb-6">User Management</h1>

    <div class="bg-slate-800 rounded-lg p-6 shadow-lg">
      <!-- User Creation Form -->
      <div class="mb-8 p-4 bg-slate-700 rounded-lg">
        <h2 class="text-xl font-semibold text-white mb-4">Create New User</h2>
        <form @submit.prevent="createUser" class="space-y-4">
          <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-1"
                >Username</label
              >
              <input
                v-model="newUser.username"
                type="text"
                required
                class="w-full px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="Enter username"
              />
            </div>
            <div>
              <label class="block text-sm font-medium text-gray-300 mb-1"
                >Password</label
              >
              <input
                v-model="newUser.password"
                type="password"
                required
                class="w-full px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                placeholder="Enter password"
              />
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-300 mb-1"
              >Groups</label
            >
            <div class="flex flex-wrap gap-2">
              <div
                v-for="group in groupStore.groups"
                :key="group.name"
                class="flex items-center space-x-2"
              >
                <input
                  type="checkbox"
                  :id="'group-' + group.name"
                  :value="group.name"
                  v-model="newUser.groups"
                  class="rounded bg-slate-600 border-slate-500 text-blue-500 focus:ring-blue-500"
                />
                <label :for="'group-' + group.name" class="text-gray-300">{{
                  group.name
                }}</label>
              </div>
            </div>
          </div>
          <div>
            <button
              type="submit"
              :disabled="userStore.loading"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 transition-colors"
            >
              {{ userStore.loading ? "Creating..." : "Create User" }}
            </button>
          </div>
          <div v-if="userStore.error" class="mt-2 text-red-400 text-sm">
            {{ userStore.error }}
          </div>
        </form>
      </div>

      <!-- Users Table -->
      <div>
        <h2 class="text-xl font-semibold text-white mb-4">Users</h2>
        <div v-if="userStore.loading" class="text-center py-4">
          <div class="loader">Loading...</div>
        </div>
        <div
          v-else-if="userStore.users.length === 0"
          class="text-center py-4 text-gray-400"
        >
          No users found.
        </div>
        <div v-else class="overflow-x-auto">
          <table class="min-w-full divide-y divide-slate-700">
            <thead class="bg-slate-700">
              <tr>
                <th
                  scope="col"
                  class="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider"
                >
                  Username
                </th>
                <th
                  scope="col"
                  class="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider"
                >
                  Groups
                </th>
                <th
                  scope="col"
                  class="px-6 py-3 text-left text-xs font-medium text-gray-300 uppercase tracking-wider"
                >
                  Actions
                </th>
              </tr>
            </thead>
            <tbody class="bg-slate-800 divide-y divide-slate-700">
              <tr v-for="user in userStore.users" :key="user.username">
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                  {{ user.username }}
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm text-gray-300">
                  <div class="flex flex-wrap gap-1">
                    <span
                      v-for="group in user.groups"
                      :key="group"
                      class="px-2 py-1 rounded-full text-xs bg-blue-900 text-blue-300"
                    >
                      {{ group }}
                    </span>
                  </div>
                </td>
                <td class="px-6 py-4 whitespace-nowrap text-sm font-medium">
                  <button
                    @click="openChangePasswordModal(user)"
                    class="text-indigo-400 hover:text-indigo-300 mr-3"
                  >
                    Change Password
                  </button>
                  <button
                    @click="confirmDeleteUser(user)"
                    class="text-red-400 hover:text-red-300"
                  >
                    Delete
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Change Password Modal -->
    <div
      v-if="passwordModal.isOpen"
      class="fixed inset-0 z-50 overflow-auto bg-black bg-opacity-50 flex justify-center items-center"
    >
      <div class="bg-slate-800 p-6 rounded-lg shadow-lg max-w-md w-full">
        <h3 class="text-lg font-medium text-white mb-4">
          Change Password for {{ passwordModal.user?.username }}
        </h3>
        <form @submit.prevent="changePassword">
          <div class="mb-4">
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
          <div class="flex justify-end space-x-3">
            <button
              type="button"
              @click="passwordModal.isOpen = false"
              class="px-4 py-2 bg-slate-600 hover:bg-slate-700 text-white rounded-md shadow-sm"
            >
              Cancel
            </button>
            <button
              type="submit"
              :disabled="userStore.loading"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md shadow-sm"
            >
              Save
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Delete User Confirmation Modal -->
    <div
      v-if="deleteModal.isOpen"
      class="fixed inset-0 z-50 overflow-auto bg-black bg-opacity-50 flex justify-center items-center"
    >
      <div class="bg-slate-800 p-6 rounded-lg shadow-lg max-w-md w-full">
        <h3 class="text-lg font-medium text-white mb-4">Delete User</h3>
        <p class="text-gray-300 mb-4">
          Are you sure you want to delete the user "{{
            deleteModal.user?.username
          }}"? This action cannot be undone.
        </p>
        <div class="flex justify-end space-x-3">
          <button
            @click="deleteModal.isOpen = false"
            class="px-4 py-2 bg-slate-600 hover:bg-slate-700 text-white rounded-md shadow-sm"
          >
            Cancel
          </button>
          <button
            @click="deleteUser"
            :disabled="userStore.loading"
            class="px-4 py-2 bg-red-600 hover:bg-red-700 text-white rounded-md shadow-sm"
          >
            Delete
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from "vue";
import { useUserStore } from "@/stores/userStore";
import { useGroupStore } from "@/stores/groupStore";

const userStore = useUserStore();
const groupStore = useGroupStore();

// New user form
const newUser = reactive({
  username: "",
  password: "",
  groups: [],
});

// Modal states
const passwordModal = reactive({
  isOpen: false,
  user: null,
  newPassword: "",
});

const deleteModal = reactive({
  isOpen: false,
  user: null,
});

// Load data on component mount
onMounted(async () => {
  // Load users and groups
  await Promise.all([userStore.fetchUsers(), groupStore.fetchGroups()]);
});

// Create a new user
const createUser = async () => {
  try {
    await userStore.createUser({
      username: newUser.username,
      password: newUser.password,
    });

    // Add user to selected groups
    if (newUser.groups.length > 0) {
      for (const groupName of newUser.groups) {
        await groupStore.addUserToGroup(newUser.username, groupName);
      }
    }

    // Reset form
    newUser.username = "";
    newUser.password = "";
    newUser.groups = [];

    // Refresh users list
    await userStore.fetchUsers();
  } catch (error) {
    console.error("Error creating user:", error);
  }
};

// Open the change password modal
const openChangePasswordModal = (user) => {
  passwordModal.user = user;
  passwordModal.newPassword = "";
  passwordModal.isOpen = true;
};

// Change user password
const changePassword = async () => {
  if (!passwordModal.user) return;

  try {
    await userStore.updateUserPassword(
      passwordModal.user.username,
      passwordModal.newPassword,
    );

    // Close modal
    passwordModal.isOpen = false;
  } catch (error) {
    console.error("Error changing password:", error);
  }
};

// Open the delete user confirmation modal
const confirmDeleteUser = (user) => {
  deleteModal.user = user;
  deleteModal.isOpen = true;
};

// Delete user
const deleteUser = async () => {
  if (!deleteModal.user) return;

  try {
    await userStore.deleteUser(deleteModal.user.username);
    deleteModal.isOpen = false;
  } catch (error) {
    console.error("Error deleting user:", error);
  }
};
</script>

<style scoped>
.loader {
  @apply inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent align-[-0.125em] text-blue-500 motion-reduce:animate-[spin_1.5s_linear_infinite];
}
</style>
