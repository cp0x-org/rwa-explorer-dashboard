import { defineConfig, devices } from '@playwright/test';

// E2E against the production build: `pnpm build` first (or `pnpm test:e2e` which builds when dist/ is missing).
// The static server behaves like the Dockerfile's `serve dist` (clean URLs, real 404s, no SPA fallback).
const PORT = 4321;

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  retries: 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: `http://localhost:${PORT}`,
    trace: 'retain-on-failure'
  },
  webServer: {
    command: `node scripts/serve-dist.mjs ${PORT}`,
    url: `http://localhost:${PORT}/assets`,
    reuseExistingServer: !process.env.CI,
    timeout: 30_000
  },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    { name: 'mobile', use: { ...devices['Pixel 7'] }, testMatch: /a11y|i18n/ }
  ]
});
