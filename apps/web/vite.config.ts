import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const apiProxyTarget = 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    proxy: {
      '/v1': {
        target: apiProxyTarget,
        ws: true,
      },
      '/health': apiProxyTarget,
      '/readyz': apiProxyTarget,
      '/healthz': apiProxyTarget,
    },
  },
});
