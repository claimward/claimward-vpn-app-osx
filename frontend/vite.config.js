import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// The build output is embedded into the Go binary via
// internal/uiserver (go:embed all:dist) and served on the loopback UI server.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/uiserver/dist',
    emptyOutDir: true,
  },
})
