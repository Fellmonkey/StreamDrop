/** @type {import('tailwindcss').Config} */
export default {
  darkMode: ['class'],
  // Touch devices must not keep a faked :hover after a tap.
  future: {
    hoverOnlyWhenSupported: true,
  },
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        canvas: '#090A0F',
        surface: '#11131B',
        elevated: '#151823',
        border: 'rgba(255,255,255,0.10)',
        accent: {
          DEFAULT: '#38BDF8',
          muted: '#0EA5E9',
        },
        success: '#34D399',
        danger: '#F87171',
        warn: '#FBBF24',
      },
      fontFamily: {
        sans: ['Inter', 'Geist Sans', 'Segoe UI', 'system-ui', 'sans-serif'],
        mono: ['JetBrains Mono', 'Geist Mono', 'ui-monospace', 'SFMono-Regular', 'monospace'],
      },
      borderRadius: {
        xl: '0.875rem',
        '2xl': '1.125rem',
      },
      keyframes: {
        'fade-up': {
          '0%': { opacity: '0', transform: 'translateY(6px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
        'pulse-soft': {
          '0%, 100%': { opacity: '1' },
          '50%': { opacity: '0.45' },
        },
        dash: {
          to: { strokeDashoffset: '-24' },
        },
      },
      animation: {
        'fade-up': 'fade-up 180ms cubic-bezier(0.16, 1, 0.3, 1) both',
        'pulse-soft': 'pulse-soft 2.4s ease-in-out infinite',
        dash: 'dash 900ms linear infinite',
      },
    },
  },
  plugins: [],
}
