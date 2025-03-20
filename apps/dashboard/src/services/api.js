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
};
