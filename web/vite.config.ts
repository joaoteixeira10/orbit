import { fileURLToPath, URL } from 'node:url'
import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
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

// Excalidraw (drawings) loads its fonts from window.EXCALIDRAW_ASSET_PATH; they
// are served from this origin as /excalidraw-assets/fonts/… (the CSP allows
// fonts only from 'self', and drawings shouldn't call a CDN). Xiaolai, the
// 12 MB Chinese hand-writing font, is left out: Chinese text falls back to a
// system font.
function excalidrawFonts(): Plugin {
  const root = fileURLToPath(new URL('./node_modules/@excalidraw/excalidraw/dist/prod/fonts', import.meta.url))
  const files = (): string[] => {
    if (!existsSync(root)) return []
    const out: string[] = []
    const walk = (dir: string) => {
      for (const name of readdirSync(dir)) {
        const p = join(dir, name)
        if (statSync(p).isDirectory()) {
          if (name !== 'Xiaolai') walk(p)
        } else if (name.endsWith('.woff2')) out.push(p)
      }
    }
    walk(root)
    return out
  }
  return {
    name: 'orbit-excalidraw-fonts',
    configureServer(server) {
      server.middlewares.use('/excalidraw-assets/fonts', (req, res, next) => {
        const p = join(root, decodeURIComponent((req.url || '').split('?')[0]))
        if (!p.startsWith(root + sep) || !p.endsWith('.woff2') || !existsSync(p)) return next()
        res.setHeader('Content-Type', 'font/woff2')
        res.end(readFileSync(p))
      })
    },
    generateBundle() {
      for (const f of files()) {
        this.emitFile({ type: 'asset', fileName: 'excalidraw-assets/fonts/' + relative(root, f).split(sep).join('/'), source: readFileSync(f) })
      }
    },
  }
}

// In dev, the SPA runs on :5173 and proxies /api to the Go server on :8080.
// In prod the Go binary serves the built SPA directly, so no proxy is needed.
// The UI shows the server's version (/about); the SPA doesn't bake its own.
export default defineConfig({
  plugins: [vue(), scalar(), excalidrawFonts()],
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
  // Excalidraw (with React) is its own lazily loaded chunk too, loaded only
  // when a drawing opens.
  build: { chunkSizeWarningLimit: 600 },
  // `npm test` runs Vitest in a jsdom environment (browser globals like
  // localStorage are available to the module graph under test).
  test: {
    environment: 'jsdom',
    include: ['src/**/*.spec.ts'],
  },
})
