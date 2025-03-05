/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{vue,js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        primary: "#0ea5e9",
        secondary: "#64748b",
        accent: "#6366f1",
        neutral: "#1f2937",
        "base-100": "#f3f4f6",
        info: "#3b82f6",
        success: "#22c55e",
        warning: "#f59e0b",
        error: "#ef4444",
      },
    },
  },
  plugins: [require("daisyui")],
  daisyui: {
    themes: [
      {
        light: {
          ...require("daisyui/src/theming/themes")["light"],
          primary: "#0ea5e9",
          secondary: "#64748b",
          accent: "#6366f1",
          neutral: "#1f2937",
          "base-100": "#f3f4f6",
        },
        dark: {
          ...require("daisyui/src/theming/themes")["dark"],
          primary: "#0ea5e9",
          secondary: "#64748b",
          accent: "#6366f1",
          neutral: "#1f2937",
          "base-100": "#0f172a",
        },
      },
    ],
  },
};
