import { fileURLToPath, URL } from 'node:url'
import { existsSync, readFileSync } from 'node:fs'
import { defineConfig, type Plugin } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// The API reference (/api/docs, served by the Go API) runs Scalar's standalone
// script from this origin — the CSP allows only script-src 'self'. Serve it in
// dev and emit it with the build as /scalar/standalone.js.
function scalar(): Plugin {
  // Not in the package's exports map, so point at the file.
  const file = fileURLToPath(new URL('./node_modules/@scalar/api-reference/dist/browser/standalone.js', import.meta.url))
  return {
    name: 'lb-scalar',
    configureServer(server) {
      server.middlewares.use('/scalar/standalone.js', (_req, res) => {
        res.setHeader('Content-Type', 'text/javascript; charset=utf-8')
        if (!existsSync(file)) {
          // e.g. the dev container's node_modules predates the dependency.
          res.statusCode = 503
          res.end('document.body.textContent = "Scalar is not installed: run npm install (make dev restarts it)."')
          return
        }
        res.end(readFileSync(file))
      })
    },
    generateBundle() {
      this.emitFile({ type: 'asset', fileName: 'scalar/standalone.js', source: readFileSync(file) })
    },
  }
}

// In dev, the SPA runs on :5173 and proxies /api to the Go server on :8080.
// In prod the Go binary serves the built SPA directly, so no proxy is needed.
// The UI shows the server's version (/about); the SPA doesn't bake its own.
export default defineConfig({
  plugins: [vue(), scalar()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // host:true binds 0.0.0.0 so the dev server is reachable when containerized.
    host: true,
    port: 5173,
    proxy: {
      '/api': {
        // Defaults to the local API; in the dev container it points at the
        // `api` service (VITE_PROXY_TARGET=http://api:8080).
        target: process.env.VITE_PROXY_TARGET || 'http://localhost:8080',
        changeOrigin: true,
      },
    },
    // Bind-mounts on Docker Desktop (macOS/Windows) don't deliver inotify
    // events, so opt into polling when running in the container.
    watch: process.env.VITE_POLL ? { usePolling: true, interval: 300 } : undefined,
  },
  // The page editor (TipTap + ProseMirror) is one lazily loaded chunk of
  // ~500 kB (160 kB gzipped), loaded only when a page opens.
  build: { chunkSizeWarningLimit: 600 },
  // `npm test` runs Vitest in a jsdom environment (browser globals like
  // localStorage are available to the module graph under test).
  test: {
    environment: 'jsdom',
    include: ['src/**/*.spec.ts'],
  },
})
