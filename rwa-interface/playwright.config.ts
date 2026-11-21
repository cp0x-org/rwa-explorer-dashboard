import { defineConfig, devices } from '@playwright/test';

// E2E against the production build (`pnpm build` first) with the real server: server/index.mjs rendering from the mock
// API (mock-api/server.mjs, build 1 = public/data/rwa.json). A second pair, the mock with --fail and a server on it,
// is for the "Data unavailable" tests. Ports can be changed with the environment (e2e/unavailable.spec.ts reads them).
const PORT = Number(process.env.E2E_PORT ?? 4321);
const API_PORT = Number(process.env.E2E_API_PORT ?? 8180);
const DOWN_PORT = Number(process.env.E2E_DOWN_PORT ?? 4322);
const DOWN_API_PORT = Number(process.env.E2E_DOWN_API_PORT ?? 8181);
const SITE_URL = 'https://rwa.cp0x.com';
const reuse = !process.env.CI;

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
  webServer: [
    { command: `node mock-api/server.mjs --port ${API_PORT}`, port: API_PORT, reuseExistingServer: reuse, timeout: 30_000 },
    {
      command: 'node server/index.mjs',
      env: { NODE_ENV: 'production', PORT: String(PORT), API_URL: `http://127.0.0.1:${API_PORT}`, SITE_URL },
      url: `http://localhost:${PORT}/healthz`,
      reuseExistingServer: reuse,
      timeout: 30_000
    },
    { command: `node mock-api/server.mjs --port ${DOWN_API_PORT} --fail`, port: DOWN_API_PORT, reuseExistingServer: reuse, timeout: 30_000 },
    {
      command: 'node server/index.mjs',
      env: { NODE_ENV: 'production', PORT: String(DOWN_PORT), API_URL: `http://127.0.0.1:${DOWN_API_PORT}`, SITE_URL },
      port: DOWN_PORT, // /healthz answers 503 on this one, by design
      reuseExistingServer: reuse,
      timeout: 30_000
    }
  ],
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] } },
    // the phone runs the layout-sensitive specs: accessibility, language, and the rights tables that scroll sideways
    { name: 'mobile', use: { ...devices['Pixel 7'] }, testMatch: /a11y|i18n|rights/ }
  ]
});
