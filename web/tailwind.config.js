/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        shell: {
          bg: 'var(--shell-bg)',
          panel: 'var(--shell-panel)',
          border: 'var(--shell-border)',
          accent: 'var(--shell-accent)',
          muted: 'var(--shell-muted)',
          text: 'var(--shell-text)',
          active: 'var(--shell-active)',
          hover: 'var(--shell-hover)',
          'user-bg': 'var(--shell-user-bg)',
          'user-border': 'var(--shell-user-border)',
        },
      },
    },
  },
  plugins: [],
}
