import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { copyFileSync, existsSync } from 'node:fs';

// The site's own images (icon, hero scenes) live in site/public and are served
// at the root by Vite natively. The README's splash-screen stays in docs/assets.

export default defineConfig({
  base: '/',
  plugins: [react()],
  // Allow importing the repo-root docs/ markdown from inside site/.
  // Dev port is overridable via TW_DEV_PORT (vite-react-ssg dev ignores --port).
  server: {
    fs: { allow: ['..'] },
    ...(process.env.TW_DEV_PORT ? { port: Number(process.env.TW_DEV_PORT), strictPort: true } : {}),
  },
  // Bundle ESM-only markdown libs for the SSG (build-time) render.
  ssr: { noExternal: ['react-markdown', 'remark-gfm', 'rehype-highlight'] },
  ssgOptions: {
    dirStyle: 'nested',
    onFinished() {
      // GitHub Pages serves 404.html for any unmatched path; mirror our /404 route there.
      if (existsSync('dist/404/index.html')) copyFileSync('dist/404/index.html', 'dist/404.html');
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.js'],
  },
});
