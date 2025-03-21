import { defineStore } from "pinia";
import { apiService } from "@/services/api";

export const useUserStore = defineStore("user", {
  state: () => ({
    users: [],
    loading: false,
    error: null,
  }),

  getters: {
    getUserByUsername: (state) => (username) => {
      return state.users.find((user) => user.username === username);
    },
  },

  actions: {
    async fetchUsers() {
      this.loading = true;
      this.error = null;

      try {
        const users = await apiService.getUsers();
        this.users = users;
        return users;
      } catch (error) {
        console.error("Error fetching users:", error);
        this.error = error.message || "Failed to fetch users";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async createUser(userData) {
      this.loading = true;
      this.error = null;

      try {
        await apiService.createUser(userData);
        // Refresh the user list after creating a new user
        await this.fetchUsers();
        return true;
      } catch (error) {
        console.error("Error creating user:", error);
        this.error = error.message || "Failed to create user";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async deleteUser(username) {
      this.loading = true;
      this.error = null;

      try {
        await apiService.deleteUser(username);
        // Update local state after successful deletion
        this.users = this.users.filter((user) => user.username !== username);
        return true;
      } catch (error) {
        console.error(`Error deleting user ${username}:`, error);
        this.error = error.message || "Failed to delete user";
        throw error;
      } finally {
        this.loading = false;
      }
    },

    async updateUserPassword(username, newPassword, currentPassword) {
      this.loading = true;
      this.error = null;

      try {
        // If currentPassword is provided, use changeOwnPassword method
        if (currentPassword) {
          await apiService.changeOwnPassword(
            username,
            newPassword,
            currentPassword,
          );
        } else {
          // Otherwise use admin updateUserPassword method
          await apiService.updateUserPassword(username, newPassword);
        }
        return true;
      } catch (error) {
        console.error(`Error updating password for user ${username}:`, error);
        this.error = error.message || "Failed to update user password";
        throw error;
      } finally {
        this.loading = false;
      }
    },
  },
});
