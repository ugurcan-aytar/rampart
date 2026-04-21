import { defineConfig, devices } from '@playwright/test';

/**
 * Playwright configuration for rampart end-to-end tests.
 *
 * The demo stack is expected to already be up before the tests run
 * (`make demo-axios` leaves it running). The Backstage IncidentDashboard
 * ships as a containerized service in the compose stack — production
 * image, not the dev harness.
 */
export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  // Deterministic: no retries. The Dockerfile prune-stage refactor
  // dropped the image from 1.23 GiB to ~770 MiB and cold boot to
  // ~1 s on a fresh runner; the previous retry was masking
  // image-weight problems that now belong to the past. A flake
  // here is a real regression and we want to see it.
  retries: 0,
  // Single worker + fullyParallel off because native-fallback.spec.ts
  // owns a destructive `docker compose down -v` at test end. Running
  // it in parallel with axios-flow.spec.ts tears the Backstage
  // container out from under the browser mid-test. Serial execution
  // is cheap — the whole suite finishes in ~30 s.
  workers: 1,
  fullyParallel: false,
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
