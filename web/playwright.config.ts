import { defineConfig, devices } from '@playwright/test'

// E2E runs against a full instance (the single binary serves the SPA + API).
// Point E2E_BASE_URL at a running server; CI builds the binary, starts it on
// :8080 with a fresh sqlite DB + known admin creds, then runs these specs.
const baseURL = process.env.E2E_BASE_URL || 'http://localhost:8080'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
})
