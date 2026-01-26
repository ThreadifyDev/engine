import type { Config } from "tailwindcss";

export default {
  content: ["./app/**/{**,.client,.server}/**/*.{js,jsx,ts,tsx}"],
  theme: {
    extend: {
      colors: {
        background: "#FFFFFF",
        foreground: "#000000",
        primary: "#000000",
        secondary: "#FFFFFF",
        muted: "#F5F5F5",
        border: "#E5E5E5",
      },
      fontFamily: {
        sans: ["Block", "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [],
} satisfies Config;
