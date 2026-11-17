import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import tailwindcss from '@tailwindcss/vite';
const proxy = {
  '/auth': { target: 'http://127.0.0.1:8080', changeOrigin: false },
  '/api': { target: 'http://127.0.0.1:8080', changeOrigin: false }
};
export default defineConfig({ plugins: [tailwindcss(), sveltekit({ adapter: adapter() })], server: { proxy, port: 5173, strictPort: true }, preview: { proxy } });
