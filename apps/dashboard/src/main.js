import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import "./assets/tailwind.css";
import "./services/axios"; // Import axios configuration

// Create the app instance
const app = createApp(App);

// Add Pinia to the app
const pinia = createPinia();
app.use(pinia);

// Mount the app
app.mount("#app");
