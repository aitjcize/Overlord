import axios from "axios";
import { useAuthStore } from "@/stores/authStore";

// Configure axios to include JWT token from localStorage
axios.interceptors.request.use((config) => {
  const token = localStorage.getItem("token");
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// Add response interceptor to handle authentication errors
axios.interceptors.response.use(
  (response) => {
    return response;
  },
  (error) => {
    // Handle 401 Unauthorized errors
    if (error.response && error.response.status === 401) {
      // Extract error message from standardized response format
      let errorMessage = "Authentication failed";
      if (error.response.data?.status === "error") {
        errorMessage = error.response.data.data;
      }
      console.error(errorMessage, error.response.data);

      try {
        // Get the auth store and log out the user
        const authStore = useAuthStore();

        // Only log out if the user is currently authenticated
        // This prevents an infinite loop if the error occurs during login
        if (authStore.isAuthenticated) {
          console.log("Unauthorized access detected. Logging out...");
          authStore.logout();

          // Force page refresh to show login screen
          // This is a simple approach that works without Vue Router
          window.location.reload();
        }
      } catch (e) {
        console.error("Error during logout process:", e);
      }
    }

    return Promise.reject(error);
  },
);

export default axios;
