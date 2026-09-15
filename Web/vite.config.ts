import react from '@vitejs/plugin-react'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [react()],
  build: {
    manifest: true,
    rolldownOptions: {
      output: {
        codeSplitting: {
          groups: [
            {
              name: 'react-vendor',
              test: /node_modules[\\/](?:react|react-dom|scheduler)[\\/]/,
              priority: 30,
            },
            {
              name: 'antd-icons',
              test: /node_modules[\\/]@ant-design[\\/](?:icons|icons-svg)[\\/]/,
              priority: 20,
              includeDependenciesRecursively: false,
            },
            {
              name: 'antd-styles',
              test: /node_modules[\\/]@ant-design[\\/](?:cssinjs|cssinjs-utils|fast-color)[\\/]/,
              priority: 19,
              includeDependenciesRecursively: false,
            },
            {
              name: 'api-client',
              test: /[\\/]src[\\/]generated[\\/]/,
              priority: 10,
            },
          ],
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: process.env.VITE_API_PROXY_TARGET ?? 'http://127.0.0.1:7580',
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      '@': new URL('./src', import.meta.url).pathname,
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['./src/**/*.test.{ts,tsx}'],
  },
})
