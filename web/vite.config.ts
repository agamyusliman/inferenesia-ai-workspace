import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'node:path'

// Vite dev server must stay in 4100–4199 (mission boundary).
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, 'src'),
    },
  },
  server: {
    port: 4150,
    strictPort: true,
    host: '127.0.0.1',
    proxy: {
      '/api': {
        target: 'http://127.0.0.1:4110',
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 4151,
    strictPort: true,
    host: '127.0.0.1',
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    chunkSizeWarningLimit: 4500,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('monaco-editor') || id.includes('@monaco-editor')) {
            return 'monaco'
          }
          if (id.includes('@excalidraw')) {
            return 'excalidraw'
          }
          if (id.includes('node_modules/mermaid') || id.includes('node_modules/dagre') || id.includes('node_modules/cytoscape')) {
            return 'mermaid'
          }
        },
      },
    },
  },
  optimizeDeps: {
    include: ['monaco-editor', '@monaco-editor/react'],
  },
})
