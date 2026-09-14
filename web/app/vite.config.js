import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { readFileSync } from 'node:fs'
// Docker build passes VITE_OUT_DIR=/out; local build defaults to ../dist
export default defineConfig({
  plugins: [react(), {
    name: 'worker-download',
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'docs/worker.js', source: readFileSync(new URL('../../workers/ingest.js', import.meta.url), 'utf8') })
    },
  }],
  build: { outDir: process.env.VITE_OUT_DIR || '../dist', emptyOutDir: true },
  server: { port: 5173, proxy: { '/api': 'http://127.0.0.1:8080', '/user_api': 'http://127.0.0.1:8080', '/admin': 'http://127.0.0.1:8080', '/open_api': 'http://127.0.0.1:8080', '/telegram': 'http://127.0.0.1:8080', '/external': 'http://127.0.0.1:8080', '/health': 'http://127.0.0.1:8080' } },
})
