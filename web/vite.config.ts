import { writeFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'

import vue from '@vitejs/plugin-vue'
import type { Plugin } from 'vite'
import { defineConfig } from 'vitest/config'

// web/dist встраивается в бинарник (go:embed), поэтому папка должна
// существовать и без сборки фронтенда: .gitkeep лежит в git и
// восстанавливается после очистки dist при сборке.
function keepDist(): Plugin {
  return {
    name: 'kintion-keep-dist',
    apply: 'build',
    closeBundle() {
      writeFileSync(fileURLToPath(new URL('./dist/.gitkeep', import.meta.url)), '')
    },
  }
}

// демо-сборка (VITE_DEMO=1, src/demo): без сервера, с относительными путями
const demo = process.env.VITE_DEMO === '1'

export default defineConfig({
  base: demo ? './' : '/',
  plugins: [vue(), keepDist()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
      '@i18n': fileURLToPath(new URL('../i18n', import.meta.url)),
      '@testdata': fileURLToPath(new URL('../testdata', import.meta.url)),
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    assetsDir: 'assets',
    // лицензионные комментарии библиотек (/*! */, @license) остаются в JS (F-91)
    rolldownOptions: {
      output: { comments: { legal: true } },
    },
  },
  server: {
    // языковые файлы лежат вне web/ (общие с сервером)
    fs: { allow: ['..'] },
    proxy: {
      '/api': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
})
