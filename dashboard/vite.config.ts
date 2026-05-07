import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      '/api/v1/sigma': {
        target: 'http://192.168.157.1:30080',
        changeOrigin: true,
        ws: true,
      },
      '/api/v1': {
        target: 'http://192.168.157.1:30082',
        changeOrigin: true,
        ws: true,
      }
    }
  }
})
