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

        // Handle the standardized response format
        if (response.data?.status !== "success" || !response.data?.data) {
          throw new Error("Invalid response format from server");
        }

        const { token } = response.data.data;

        // Store token in localStorage and state
        localStorage.setItem("token", token);
        this.token = token;

        // Set user data
        this.user = { username };

        return true;
      } catch (error) {
        console.error("Login failed:", error);

        // Extract error message from standardized response format
        if (error.response?.data?.status === "error") {
          this.error = error.response.data.data;
        } else {
          this.error = "Login failed. Please check your credentials.";
        }

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

        try {
          // Extract user info from JWT token
          // JWT tokens are in the format: header.payload.signature
          const payload = token.split(".")[1];
          if (payload) {
            // Decode the base64 payload
            const decodedPayload = JSON.parse(atob(payload));
            if (decodedPayload.username) {
              this.user = { username: decodedPayload.username };
              return;
            }
          }
        } catch (error) {
          console.error("Error decoding JWT token:", error);
        }

        // Fallback to default user if token parsing fails
        this.user = { username: "User" };
      }
    },
  },
});
