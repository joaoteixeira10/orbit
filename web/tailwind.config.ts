import type { Config } from 'tailwindcss'

// Colors are CSS-variable-backed so light/dark switch by toggling a class on
// <html>. The palette is Apple's system colours (see src/styles/main.css).
export default {
  darkMode: 'class',
  content: ['./index.html', './src/**/*.{vue,ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: 'var(--bg)',
        grouped: 'var(--grouped)',
        surface: 'var(--surface)',
        'surface-2': 'var(--surface-2)',
        'surface-3': 'var(--surface-3)',
        'surface-hover': 'var(--surface-hover)',
        fill: 'var(--fill)',
        'fill-2': 'var(--fill-2)',
        'fill-strong': 'var(--fill-strong)',
        text: 'var(--text)',
        'text-2': 'var(--text-2)',
        'text-3': 'var(--text-3)',
        'text-faint': 'var(--text-faint)',
        border: 'var(--border)',
        'border-2': 'var(--border-2)',
        'border-strong': 'var(--border-strong)',
        accent: 'var(--accent)',
        'accent-strong': 'var(--accent-strong)',
        'accent-soft': 'var(--accent-soft)',
        'accent-bg': 'var(--accent-bg)',
        'accent-text': 'var(--accent-text)',
        'accent-fg': 'var(--accent-fg)', // text on the accent (dark on Ink's bone)
        ws: 'var(--ws)',
        side: 'var(--side)',
        'line-soft': 'var(--line-soft)',
        ok: 'var(--ok)',
        'ok-soft': 'var(--ok-soft)',
        'ok-bg': 'var(--ok-bg)',
        'ok-text': 'var(--ok-text)',
        warn: 'var(--warn)',
        'warn-soft': 'var(--warn-soft)',
        'warn-bg': 'var(--warn-bg)',
        'warn-text': 'var(--warn-text)',
        crit: 'var(--crit)',
        'crit-bg': 'var(--crit-bg)',
        'crit-text': 'var(--crit-text)',
        'crit-border': 'var(--crit-border)',
        danger: 'var(--danger)',
        'danger-soft': 'var(--danger-soft)',
        violet: 'var(--violet)',
        'violet-soft': 'var(--violet-soft)',
      },
      fontFamily: {
        sans: ['-apple-system', 'BlinkMacSystemFont', '"SF Pro Text"', 'Inter', '"Segoe UI"', 'Roboto', 'sans-serif'],
        display: ['-apple-system', 'BlinkMacSystemFont', '"SF Pro Display"', 'Inter', '"Segoe UI"', 'Roboto', 'sans-serif'],
        mono: ['ui-monospace', '"SF Mono"', 'SFMono-Regular', 'Menlo', 'Consolas', 'monospace'],
      },
      boxShadow: {
        soft: 'var(--shadow)',
        lg2: 'var(--shadow-lg)',
      },
      borderRadius: {
        // Controls 9px, cards 12px, sheets 16px.
        lg: '9px',
        xl: '12px',
        '2xl': '16px',
      },
    },
  },
  plugins: [],
} satisfies Config
