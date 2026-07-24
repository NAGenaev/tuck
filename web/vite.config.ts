// SPDX-License-Identifier: AGPL-3.0-only
// Forked from github.com/remnawave/frontend (commit 9d671520067f73b2beb96c282f2ce2ff7b7a9a00), adapted for Tuck.
import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'
import tsconfigPaths from 'vite-tsconfig-paths'

export default defineConfig({
    base: '/ui/',
    plugins: [react(), tsconfigPaths()],
    build: {
        target: 'esnext',
        outDir: '../internal/ui/assets',
        emptyOutDir: true,
        chunkSizeWarningLimit: 1000000
    },
    server: {
        host: '0.0.0.0',
        port: 3333,
        proxy: {
            '/v1': 'http://localhost:8200',
            '/v2': 'http://localhost:8200',
            '/openapi.json': 'http://localhost:8200',
            '/metrics': 'http://localhost:8200'
        }
    }
})
