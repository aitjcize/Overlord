import axios from "axios";

const API_BASE_URL = "/api";

export const apiService = {
  async getClients() {
    try {
      const response = await axios.get(`${API_BASE_URL}/agents`);
      return response.data;
    } catch (error) {
      console.error("Error fetching clients:", error);
      // Propagate the error so it can be handled by the caller
      throw error;
    }
  },

  async getClientProperties(mid) {
    try {
      const response = await axios.get(
        `${API_BASE_URL}/agents/${mid}/properties`,
      );
      return response.data;
    } catch (error) {
      console.error(`Error fetching properties for client ${mid}:`, error);
      // Propagate the error so it can be handled by the caller
      throw error;
    }
  },

  async upgradeClients() {
    try {
      const response = await axios.post(`${API_BASE_URL}/agents/upgrade`);
      return response.data;
    } catch (error) {
      console.error("Error triggering client upgrade:", error);
      // Propagate the error so it can be handled by the caller
      throw error;
    }
  },
};
