/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ["./app/**/*.{ts,tsx}", "./components/**/*.{ts,tsx}", "./lib/**/*.{ts,tsx}", "./hooks/**/*.{ts,tsx}", "./contexts/**/*.{ts,tsx}"],
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--font-geist)', 'sans-serif'],
        mono: ['var(--font-geist-mono)', 'monospace'],
      },
    },
    borderRadius: {
      none: "0",
      DEFAULT: "3px",
      sm: "2px",
      md: "4px",
      lg: "6px",
      xl: "8px",
      "2xl": "10px",
      "3xl": "12px",
      full: "9999px",
    },
  },
  plugins: [],
};
