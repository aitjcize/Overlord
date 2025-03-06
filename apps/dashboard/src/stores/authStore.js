import { defineStore } from "pinia";
import axios from "@/services/axios";
import { monitorService } from "@/services/monitor";

export const useAuthStore = defineStore("auth", {
  state: () => ({
    // User data
    user: null,
    // JWT token
    token: localStorage.getItem("token") || null,
    // Login loading state
    loading: false,
    // Login error message
    error: null,
  }),

  getters: {
    // Check if user is authenticated
    isAuthenticated: (state) => !!state.token,

    // Get the authenticated user
    getUser: (state) => state.user,

    // Get authentication headers for API requests
    authHeaders: (state) => {
      return state.token
        ? {
            Authorization: `Bearer ${state.token}`,
          }
        : {};
    },
  },

  actions: {
    // Login user with username and password
    async login(username, password) {
      this.loading = true;
      this.error = null;

      try {
        const response = await axios.post("/api/auth/login", {
          username,
          password,
        });

        const { token } = response.data;

        // Store token in localStorage and state
        localStorage.setItem("token", token);
        this.token = token;

        // Set user data
        this.user = { username };

        return true;
      } catch (error) {
        console.error("Login failed:", error);
        this.error =
          error.response?.data?.error ||
          "Login failed. Please check your credentials.";
        return false;
      } finally {
        this.loading = false;
      }
    },

    // Logout user
    logout() {
      // Stop the monitor service
      monitorService.stop();

      // Clear token from localStorage and state
      localStorage.removeItem("token");
      this.token = null;
      this.user = null;
    },

    // Check if token is valid (can be extended to verify expiration)
    checkAuth() {
      return !!this.token;
    },

    // Initialize auth from localStorage
    initAuth() {
      const token = localStorage.getItem("token");
      if (token) {
        this.token = token;

        // You could also verify the token with the server here
        // or decode the JWT to get user info

        // For simplicity, we'll just set a basic user object
        this.user = { username: "User" };
      }
    },
  },
});
