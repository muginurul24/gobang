import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

const apiProxyTarget =
  process.env.VITE_API_PROXY_TARGET || 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [tailwindcss(), sveltekit()],
  server: {
    proxy: {
      '/v1': {
        target: apiProxyTarget,
        changeOrigin: true,
        secure: false,
        ws: true,
      },
      '/health': {
        target: apiProxyTarget,
        changeOrigin: true,
        secure: false,
      },
      '/readyz': {
        target: apiProxyTarget,
        changeOrigin: true,
        secure: false,
      },
      '/healthz': {
        target: apiProxyTarget,
        changeOrigin: true,
        secure: false,
      },
    },
  },
});
