import { createApp } from "vue";
import { createPinia } from "pinia";
import App from "./App.vue";
import "./assets/tailwind.css";
import "./assets/mobile-viewport-fix.css"; // Import mobile viewport fix
import "./services/axios"; // Import axios configuration

// Fix for mobile Safari viewport height
const setVhVariable = () => {
  // First we get the viewport height and multiply it by 1% to get a value for a vh unit
  const vh = window.innerHeight * 0.01;
  // Then we set the value in the --vh custom property to the root of the document
  document.documentElement.style.setProperty("--vh", `${vh}px`);

  // Add a small delay for iOS "Add to Home Screen" mode to ensure correct height
  if (navigator.standalone) {
    setTimeout(() => {
      const vh = window.innerHeight * 0.01;
      document.documentElement.style.setProperty("--vh", `${vh}px`);
    }, 100);
  }
};

// Set the height variable on initial load
setVhVariable();

// Update the height variable on resize and orientation change
window.addEventListener("resize", setVhVariable);
window.addEventListener("orientationchange", () => {
  // Add a small delay after orientation change to get the correct height
  setTimeout(setVhVariable, 100);
});

// Additional event for iOS "Add to Home Screen" mode
if ("standalone" in navigator && navigator.standalone) {
  // iOS web app is running in "Add to Home Screen" mode
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      setVhVariable();
    }
  });
}

// Create the app instance
const app = createApp(App);

// Add Pinia to the app
const pinia = createPinia();
app.use(pinia);

// Mount the app
app.mount("#app");
