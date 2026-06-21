export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  theme: {
    extend: {
      colors: {
        navy: { 950: '#0a1628', 900: '#0e1d35', 800: '#13294a', 700: '#1b3563' },
        glow: { cyan: '#00e5c7', amber: '#ffb547', red: '#ff4d5e' }
      },
      fontFamily: { mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'monospace'] },
      animation: {
        'pulse-fast': 'pulse 0.8s cubic-bezier(0.4, 0, 0.6, 1) infinite'
      }
    }
  },
  plugins: []
}
