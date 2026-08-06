import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  base: "./",
  plugins: [
    vue({
      template: {
        compilerOptions: {
          isCustomElement: (tag) => tag === 'kioubit-auth-btn-window',
        }
      }
    }),
  ],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8001',
        changeOrigin: true,
      }
    }
  },
  build: {
    rolldownOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('@fortawesome')) {
            return 'fontawesome';
          }
          if (id.includes('country-flag-icons')) {
            return 'country-flag-icons';
          }
          if (id.includes('ol/') || id.includes('ol-map')) {
            return 'ol';
          }
        }
      }
    }
  },
  resolve: {
    alias: {
      "@": "/src",
    },
  }
})
