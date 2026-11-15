import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { fileURLToPath } from 'node:url'
import { defineConfig } from 'vite'

// `pnpm dev` talks to a local `troy serve` (TROY_ADDR, default 127.0.0.1:8787).
const troy = `http://${process.env.TROY_ADDR ?? '127.0.0.1:8787'}`

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: 'dist/app' },
  server: { proxy: { '/api': troy, '/s': troy, '/sign-in': troy } },
})
