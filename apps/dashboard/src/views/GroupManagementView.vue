<template>
  <div class="p-6">
    <h1 class="text-2xl font-bold text-white mb-6">Group Management</h1>

    <div class="bg-slate-800 rounded-lg p-6 shadow-lg">
      <!-- Group Creation Form -->
      <div class="mb-8 p-4 bg-slate-700 rounded-lg">
        <h2 class="text-xl font-semibold text-white mb-4">Create New Group</h2>
        <form @submit.prevent="createGroup" class="space-y-4">
          <div>
            <label class="block text-sm font-medium text-gray-300 mb-1"
              >Group Name</label
            >
            <input
              v-model="newGroup.name"
              type="text"
              required
              class="w-full px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
              placeholder="Enter group name"
            />
          </div>
          <div>
            <button
              type="submit"
              :disabled="groupStore.loading"
              class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md shadow-sm focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-opacity-50 transition-colors"
            >
              {{ groupStore.loading ? "Creating..." : "Create Group" }}
            </button>
          </div>
          <div v-if="groupStore.error" class="mt-2 text-red-400 text-sm">
            {{ groupStore.error }}
          </div>
        </form>
      </div>

      <!-- Groups Table -->
      <div>
        <div class="mb-4">
          <h2 class="text-xl font-semibold text-white">Groups</h2>
        </div>
        <div v-if="groupStore.loading" class="text-center py-4">
          <div class="loader">Loading...</div>
        </div>
        <div
          v-else-if="groupStore.groups.length === 0"
          class="text-center py-4 text-gray-400"
        >
          No groups found.
        </div>
        <div v-else class="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <div
            v-for="group in groupStore.groups"
            :key="group.name"
            class="bg-slate-700 rounded-lg overflow-hidden"
          >
            <div class="p-4 bg-slate-600 flex justify-between items-center">
              <h3 class="text-lg font-medium text-white">{{ group.name }}</h3>
              <button
                v-if="!isAdminGroup(group.name)"
                @click="confirmDeleteGroup(group)"
                class="text-red-400 hover:text-red-300"
              >
                Delete
              </button>
            </div>

            <div class="p-4">
              <h4 class="text-sm font-medium text-gray-300 mb-2">Users</h4>

              <!-- Users in this group -->
              <div class="mb-4">
                <div
                  v-if="!groupUsers[group.name]"
                  class="text-center py-2 text-gray-400 text-sm"
                >
                  Loading users...
                </div>
                <div
                  v-else-if="groupUsers[group.name].length === 0"
                  class="text-center py-2 text-gray-400 text-sm"
                >
                  No users in this group.
                </div>
                <div v-else class="space-y-2">
                  <div
                    v-for="user in groupUsers[group.name]"
                    :key="typeof user === 'string' ? user : user.username"
                    class="flex justify-between items-center bg-slate-800 p-2 rounded"
                  >
                    <span class="text-sm text-gray-300">
                      {{ typeof user === "string" ? user : user.username }}
                    </span>
                    <button
                      v-if="
                        canRemoveUserFromGroup(
                          group.name,
                          typeof user === 'string' ? user : user.username,
                        )
                      "
                      @click="
                        removeUserFromGroup(
                          group.name,
                          typeof user === 'string' ? user : user.username,
                        )
                      "
                      class="text-red-400 hover:text-red-300 text-xs"
                    >
                      Remove
                    </button>
                  </div>
                </div>
              </div>

              <!-- Add User to Group -->
              <div class="mt-4">
                <h4 class="text-sm font-medium text-gray-300 mb-2">
                  Add User to Group
                </h4>
                <div class="flex space-x-2">
                  <select
                    v-model="selectedUser[group.name]"
                    class="flex-1 px-3 py-2 bg-slate-600 border border-slate-500 rounded-md text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
                  >
                    <option value="" disabled>Select a user</option>
                    <option
                      v-for="user in availableUsersForGroup(group.name)"
                      :key="user.username"
                      :value="user.username"
                    >
                      {{ user.username }}
                    </option>
                  </select>
                  <button
                    @click="addUserToGroup(group.name)"
                    :disabled="!selectedUser[group.name]"
                    class="px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md shadow-sm disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    Add
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Delete Group Confirmation Modal -->
    <div
      v-if="deleteModal.isOpen"
      class="fixed inset-0 z-50 overflow-auto bg-black bg-opacity-50 flex justify-center items-center"
    >
      <div class="bg-slate-800 p-6 rounded-lg shadow-lg max-w-md w-full">
        <h3 class="text-lg font-medium text-white mb-4">Delete Group</h3>
        <p class="text-gray-300 mb-4">
          Are you sure you want to delete the group "{{
            deleteModal.group?.name
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
            @click="deleteGroup"
            :disabled="groupStore.loading"
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
import { ref, reactive, onMounted, computed, watch } from "vue";
import { useGroupStore } from "@/stores/groupStore";
import { useUserStore } from "@/stores/userStore";

const groupStore = useGroupStore();
const userStore = useUserStore();

// New group form
const newGroup = reactive({
  name: "",
});

// Selected user for each group
const selectedUser = reactive({});

// Group users cache
const groupUsers = reactive({});

// Delete modal state
const deleteModal = reactive({
  isOpen: false,
  group: null,
});

// Load data on component mount
onMounted(async () => {
  // Load all data
  await syncAllData();
});

// Watch for changes in groups
watch(
  () => groupStore.groups,
  async (newGroups) => {
    if (newGroups.length > 0) {
      await loadAllGroupUsers();
    }
  },
  { deep: true },
);

// Load users for all groups
const loadAllGroupUsers = async () => {
  try {
    for (const group of groupStore.groups) {
      await refreshGroupUsers(group.name);
    }
  } catch (error) {
    console.error("Error loading group users:", error);
  }
};

// Create a new group
const createGroup = async () => {
  try {
    await groupStore.createGroup({
      name: newGroup.name,
    });

    // Reset form
    newGroup.name = "";

    // Refresh groups and load users for the new group
    await groupStore.fetchGroups();
    loadAllGroupUsers();
  } catch (error) {
    console.error("Error creating group:", error);
  }
};

// Get users that are not in the specified group
const availableUsersForGroup = (groupName) => {
  if (!groupUsers[groupName]) return [];

  // Handle both string usernames and user objects
  const groupUsernames = groupUsers[groupName].map((u) =>
    typeof u === "string" ? u : u.username,
  );

  return userStore.users.filter(
    (user) => !groupUsernames.includes(user.username),
  );
};

// Force a complete refresh of all data
const syncAllData = async () => {
  try {
    // Clear any cached data
    Object.keys(groupUsers).forEach((key) => {
      groupUsers[key] = [];
    });

    // Reload all data in sequence for reliability
    await userStore.fetchUsers();
    await groupStore.fetchGroups();

    // Reload all group users
    await loadAllGroupUsers();
  } catch (error) {
    console.error("Error syncing data:", error);
  }
};

// Special admin group refresh
const refreshAdminGroup = async () => {
  const adminGroup = groupStore.getGroupByName("admin");

  if (adminGroup) {
    await refreshGroupUsers("admin");
  }
};

// Add a user to a group
const addUserToGroup = async (groupName) => {
  const username = selectedUser[groupName];
  if (!username) return;

  try {
    // Pass username first, then groupName to match the store method
    await groupStore.addUserToGroup(groupName, username);

    // Refresh group users using our enhanced method
    await refreshGroupUsers(groupName);

    // Explicitly refresh admin group if this is the admin group
    if (isAdminGroup(groupName)) {
      await refreshAdminGroup();
    }

    // Reset selected user
    selectedUser[groupName] = "";
  } catch (error) {
    console.error(`Error adding user to group ${groupName}:`, error);
  }
};

// Remove a user from a group
const removeUserFromGroup = async (groupName, username) => {
  try {
    await groupStore.removeUserFromGroup(groupName, username);

    // Refresh group users using our enhanced method
    await refreshGroupUsers(groupName);

    // Explicitly refresh admin group if this is the admin group
    if (isAdminGroup(groupName)) {
      await refreshAdminGroup();
    }
  } catch (error) {
    console.error(`Error removing user from group ${groupName}:`, error);
  }
};

// Check if a group is the admin group
const isAdminGroup = (groupName) => {
  return groupName === "admin";
};

// Open the delete group confirmation modal
const confirmDeleteGroup = (group) => {
  deleteModal.group = group;
  deleteModal.isOpen = true;
};

// Delete a group
const deleteGroup = async () => {
  if (!deleteModal.group) return;

  try {
    await groupStore.deleteGroup(deleteModal.group.name);
    deleteModal.isOpen = false;

    // Refresh groups
    await groupStore.fetchGroups();
  } catch (error) {
    console.error("Error deleting group:", error);
  }
};

// Check if a user can be removed from a group
const canRemoveUserFromGroup = (username, groupName) => {
  // Don't allow removing the last user from the admin group
  if (isAdminGroup(groupName)) {
    // Count how many users are in the admin group
    const adminUsers = groupUsers[groupName] || [];
    return adminUsers.length > 1;
  }

  // For non-admin groups, always allow removing users
  return true;
};

// Force refresh users for a group
const refreshGroupUsers = async (groupName) => {
  try {
    const users = await groupStore.fetchGroupUsers(groupName);

    // Ensure the groupUsers object is reactive and properly updated
    if (users && Array.isArray(users)) {
      // Create a new array to trigger reactivity
      groupUsers[groupName] = [...users];
    } else {
      groupUsers[groupName] = [];
    }

    return users;
  } catch (error) {
    console.error(`Error refreshing users for group ${groupName}:`, error);
    return [];
  }
};
</script>

<style scoped>
.loader {
  @apply inline-block h-8 w-8 animate-spin rounded-full border-4 border-solid border-current border-r-transparent align-[-0.125em] text-blue-500 motion-reduce:animate-[spin_1.5s_linear_infinite];
}
</style>
