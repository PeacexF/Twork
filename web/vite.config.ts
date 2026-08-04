/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// The Go backend (internal/web) embeds this build output via go:embed, which
// can't reach outside its own package directory -- so the build has to land
// inside internal/web/dist, not the default web/dist next to this config.
export default defineConfig({
  plugins: [react()],
  base: './',
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/setupTests.ts'],
  },
})
