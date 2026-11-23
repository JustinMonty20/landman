import { defineConfig } from '@playwright/test';
const port = Number(process.env.PLAYWRIGHT_PORT ?? '5174');
const origin = `http://127.0.0.1:${port}`;
export default defineConfig({
  testDir: './tests',
  use: { baseURL: origin, launchOptions: { args: ['--use-gl=angle', '--use-angle=swiftshader', '--enable-unsafe-swiftshader'] } },
  webServer: { command: `npm run build && npm run preview -- --port ${port} --strictPort`, url: origin, reuseExistingServer: false },
  projects: [
    { name: 'desktop', use: { viewport: { width: 1440, height: 1000 } } },
    { name: 'tablet', use: { viewport: { width: 768, height: 1024 }, hasTouch: true } }
  ]
});
