import axios from "axios";

const API_BASE_URL = "/api";

// Helper function to extract data from standardized response format
const extractData = (response) => {
  if (
    !response.data ||
    response.data.status !== "success" ||
    response.data.data === undefined
  ) {
    throw new Error("Invalid response format from server");
  }
  return response.data.data;
};

export const apiService = {
  async getClients() {
    try {
      const response = await axios.get(`${API_BASE_URL}/agents`);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error("Error fetching clients:", error);
      throw error;
    }
  },

  async getClientProperties(mid) {
    try {
      const response = await axios.get(
        `${API_BASE_URL}/agents/${mid}/properties`,
      );
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(`Error fetching properties for client ${mid}:`, error);
      throw error;
    }
  },

  async upgradeClients() {
    try {
      const response = await axios.post(`${API_BASE_URL}/agents/upgrade`);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error("Error triggering client upgrade:", error);
      throw error;
    }
  },

  // User management API methods
  async getUsers() {
    try {
      const response = await axios.get(`${API_BASE_URL}/users`);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error("Error fetching users:", error);
      throw error;
    }
  },

  async createUser(userData) {
    try {
      const response = await axios.post(`${API_BASE_URL}/users`, userData);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error("Error creating user:", error);
      throw error;
    }
  },

  async deleteUser(username) {
    try {
      const response = await axios.delete(`${API_BASE_URL}/users/${username}`);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(`Error deleting user ${username}:`, error);
      throw error;
    }
  },

  async updateUserPassword(username, newPassword) {
    try {
      const response = await axios.put(
        `${API_BASE_URL}/users/${username}/password`,
        {
          password: newPassword,
        },
      );
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(`Error updating password for user ${username}:`, error);
      throw error;
    }
  },

  async changeOwnPassword(username, newPassword, currentPassword) {
    try {
      const response = await axios.put(`${API_BASE_URL}/users/self/password`, {
        new_password: newPassword,
        current_password: currentPassword,
      });
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(`Error changing password for user ${username}:`, error);
      throw error;
    }
  },

  // Group management API methods
  async getGroups() {
    try {
      const response = await axios.get(`${API_BASE_URL}/groups`);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error("Error fetching groups:", error);
      throw error;
    }
  },

  async createGroup(groupData) {
    try {
      const response = await axios.post(`${API_BASE_URL}/groups`, groupData);
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error("Error creating group:", error);
      throw error;
    }
  },

  async deleteGroup(groupName) {
    try {
      const response = await axios.delete(
        `${API_BASE_URL}/groups/${groupName}`,
      );
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(`Error deleting group ${groupName}:`, error);
      throw error;
    }
  },

  async getGroupUsers(groupName) {
    try {
      const response = await axios.get(
        `${API_BASE_URL}/groups/${groupName}/users`,
      );
      const data = extractData(response);

      // Handle different response formats
      let users = [];

      if (Array.isArray(data)) {
        users = data;
      } else if (data && typeof data === "object") {
        // Try to extract users from object response
        if (Array.isArray(data.users)) {
          users = data.users;
        } else if (data.data && Array.isArray(data.data)) {
          users = data.data;
        } else {
          // Last resort - try to convert object to array if it has keys
          const possibleUsers = Object.values(data);
          if (possibleUsers.length > 0) {
            users = possibleUsers;
          }
        }
      }

      return users;
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(`Error fetching users for group ${groupName}:`, error);
      throw error;
    }
  },

  async addUserToGroup(groupName, username) {
    try {
      const response = await axios.post(
        `${API_BASE_URL}/groups/${groupName}/users`,
        {
          username,
        },
      );
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(
        `Error adding user ${username} to group ${groupName}:`,
        error,
      );
      throw error;
    }
  },

  async removeUserFromGroup(groupName, username) {
    try {
      const response = await axios.delete(
        `${API_BASE_URL}/groups/${groupName}/users/${username}`,
      );
      return extractData(response);
    } catch (error) {
      if (error.response?.data?.status === "error") {
        throw new Error(error.response.data.data);
      }
      console.error(
        `Error removing user ${username} from group ${groupName}:`,
        error,
      );
      throw error;
    }
  },
};
