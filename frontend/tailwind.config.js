/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,jsx,ts,tsx}'],
  theme: {
    extend: {
      colors: {
        crimson: '#D2042D',
        ink: '#050505',
      },
      fontFamily: {
        display: ['Orbitron', 'JetBrains Mono', 'monospace'],
        body: ['Inter', 'system-ui', 'sans-serif'],
      },
      boxShadow: {
        glow: '0 0 24px rgba(210, 4, 45, 0.35)',
      },
      animation: {
        pulseSlow: 'pulse 5s ease-in-out infinite',
      },
    },
  },
  plugins: [],
}

