import axios from "axios";

const API_BASE_URL = "/api";

export const apiService = {
  async getClients() {
    try {
      const response = await axios.get(`${API_BASE_URL}/agents/list`);
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
        `${API_BASE_URL}/agent/properties/${mid}`,
      );
      return response.data;
    } catch (error) {
      console.error(`Error fetching properties for client ${mid}:`, error);
      // Propagate the error so it can be handled by the caller
      throw error;
    }
  },
};
