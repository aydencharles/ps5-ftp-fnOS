import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: { '/api': 'http://127.0.0.1:8100', '/healthz': 'http://127.0.0.1:8100' },
  },
  test: { environment: 'jsdom' },
})
