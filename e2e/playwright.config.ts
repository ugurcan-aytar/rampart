import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for rampart end-to-end tests.
 *
 * The demo stack is expected to already be up before the tests run
 * (`make demo-axios` leaves it running). The Backstage IncidentDashboard
 * is served by the plugin dev harness — run in a second terminal:
 *
 *     yarn workspace rampart dev
 *
 * CI (Adım 8) wires a globalSetup that starts both; this config stays
 * minimal so local runs work against whatever's already on the host.
 */
export default defineConfig({
  testDir: './tests',
  timeout: 120_000,
  // CI retries once — the Backstage container boot has enough
  // moving parts (SPA static assets, plugin-app-backend mount,
  // engine proxy readiness) that a cold first-run occasionally
  // trips on an ordering nit. A retry is cheap and hides nothing
  // since traces upload anyway.
  retries: process.env.CI ? 1 : 0,
  reporter: [['list']],
  use: {
    baseURL: process.env.BACKSTAGE_URL ?? 'http://localhost:3000',
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
