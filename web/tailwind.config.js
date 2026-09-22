/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  // MUI compatibility: disable preflight (MUI CssBaseline handles base styles)
  corePlugins: {
    preflight: false,
  },
  // Allow Tailwind to override MUI styles when needed
  important: '#root',
  theme: {
    extend: {
      colors: {
        // DefensePoint Enterprise SIEM color scheme
        defense: {
          bg: {
            primary: '#0E0E0E',      // DefensePoint black
            secondary: '#1a1a1a',    // Elevated black
            elevated: '#212121',     // Card background
            card: '#1a1a1a',         // Secondary card
          },
          border: {
            primary: '#2a2a2a',      // Subtle borders
            elevated: '#333333',     // Elevated borders
            accent: '#DB2833',       // DefensePoint red
          },
          text: {
            primary: '#ffffff',      // White text
            secondary: '#a0a0a0',    // Gray text
            muted: '#707070',        // Muted text
            inverse: '#ffffff',      // White text
          },
          red: {
            primary: '#DB2833',      // DefensePoint red
            dark: '#b01f28',         // Darker red
            light: '#e63946',        // Lighter red
            glow: '#DB2833',         // Glow effect
          },
          accent: {
            success: '#10b981',      // Green
            warning: '#f59e0b',      // Amber
            danger: '#DB2833',       // DefensePoint red
            critical: '#b01f28',     // Dark red
            info: '#60a5fa',         // Blue
          },
          severity: {
            critical: '#7f1d1d',     // Very dark red
            high: '#c2410c',         // Dark orange
            medium: '#854d0e',       // Dark yellow
            low: '#1e40af',          // Dark blue
            info: '#0e7490',         // Dark cyan
          },
        },
        primary: {
          50: '#f0f9ff',
          100: '#e0f2fe',
          200: '#bae6fd',
          300: '#7dd3fc',
          400: '#38bdf8',
          500: '#0ea5e9',
          600: '#0284c7',
          700: '#0369a1',
          800: '#075985',
          900: '#0c4a6e',
        },
      },
      fontFamily: {
        mono: ['JetBrains Mono', 'Consolas', 'Monaco', 'Courier New', 'monospace'],
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
      },
      boxShadow: {
        'defense': '0 0 30px rgba(0, 0, 0, 0.8)',
        'defense-lg': '0 0 60px rgba(0, 0, 0, 0.9)',
        'glow-red': '0 0 20px rgba(219, 40, 51, 0.4)',
        'glow-red-lg': '0 0 30px rgba(219, 40, 51, 0.6)',
      },
      letterSpacing: {
        wider: '0.1em',
        widest: '0.15em',
      },
    },
  },
  plugins: [],
}
