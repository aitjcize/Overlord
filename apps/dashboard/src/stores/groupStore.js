import { defineStore } from "pinia";
import { apiService } from "@/services/api";

export const useGroupStore = defineStore("group", {
  state: () => ({
    groups: [],
    groupUsers: {}, // Map of group name to array of users
    loading: false,
    error: null,
  }),

  getters: {
    getGroupByName: (state) => (name) => {
      return state.groups.find((group) => group.name === name);
    },
    getUsersInGroup: (state) => (groupName) => {
      return state.groupUsers[groupName] || [];
    },
  },

  actions: {
    async fetchGroups() {
      this.loading = true;
      this.error = null;

      try {
        const groups = await apiService.getGroups();
        this.groups = groups;
        return groups;
      } catch (error) {
        console.error("Error fetching groups:", error);
        this.error = error.message || "Failed to fetch groups";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async fetchGroupUsers(groupName) {
      this.loading = true;
      this.error = null;

      try {
        const users = await apiService.getGroupUsers(groupName);
        // Update the groupUsers state with the fetched users
        this.groupUsers = {
          ...this.groupUsers,
          [groupName]: users,
        };
        return users;
      } catch (error) {
        console.error(`Error fetching users for group ${groupName}:`, error);
        this.error = error.message || "Failed to fetch group users";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async createGroup(groupData) {
      this.loading = true;
      this.error = null;

      try {
        await apiService.createGroup(groupData);
        // Refresh the group list after creating a new group
        await this.fetchGroups();
        return true;
      } catch (error) {
        console.error("Error creating group:", error);
        this.error = error.message || "Failed to create group";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async deleteGroup(groupName) {
      this.loading = true;
      this.error = null;

      try {
        await apiService.deleteGroup(groupName);
        // Update local state after successful deletion
        this.groups = this.groups.filter((group) => group.name !== groupName);
        // Remove the group from groupUsers
        const { [groupName]: _, ...rest } = this.groupUsers; // eslint-disable-line no-unused-vars
        this.groupUsers = rest;
        return true;
      } catch (error) {
        console.error(`Error deleting group ${groupName}:`, error);
        this.error = error.message || "Failed to delete group";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async addUserToGroup(groupName, username) {
      this.loading = true;
      this.error = null;

      try {
        await apiService.addUserToGroup(groupName, username);

        // Refresh the group users
        await this.fetchGroupUsers(groupName);
        return true;
      } catch (error) {
        console.error(
          `Error adding user ${username} to group ${groupName}:`,
          error,
        );
        this.error = error.message || "Failed to add user to group";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async removeUserFromGroup(groupName, username) {
      this.loading = true;
      this.error = null;

      try {
        await apiService.removeUserFromGroup(groupName, username);
        // Update the local state for the group's users
        if (this.groupUsers[groupName]) {
          this.groupUsers[groupName] = this.groupUsers[groupName].filter(
            (user) => {
              // Handle both string usernames and user objects
              const userIdentifier =
                typeof user === "string" ? user : user.username;
              return userIdentifier !== username;
            },
          );
        }
        return true;
      } catch (error) {
        console.error(
          `Error removing user ${username} from group ${groupName}:`,
          error,
        );
        this.error = error.message || "Failed to remove user from group";
        throw error;
      } finally {
        this.loading = false;
      }
    },
  },
});
