import axios from "axios";

const API_BASE_URL = "/api";

export const apiService = {
  async getClients() {
    const response = await axios.get(`${API_BASE_URL}/agents/list`);
    return response.data;
  },

  async getClientProperties(mid) {
    const response = await axios.get(`${API_BASE_URL}/agent/properties/${mid}`);
    return response.data;
  },
};
