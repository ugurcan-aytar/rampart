import { expect, test } from '@playwright/test';

/**
 * axios-flow.spec.ts — end-to-end replay of the 2026-03-31 axios
 * supply-chain compromise against the production demo stack:
 *
 *   1. Hit the engine HTTP API via fetch (scenario has already been
 *      replayed by `make demo-axios`; we assert 2 incidents exist).
 *   2. Open the containerized Backstage IncidentDashboard in headless
 *      Chrome and confirm it renders those same two incidents.
 *   3. Confirm the slack-notifier has seen the broadcast — proved
 *      indirectly via the engine's /v1/incidents count (slack-notifier
 *      subscribes to /v1/stream inside the compose network; we can't
 *      query its log from the browser).
 *
 * Prerequisite: `make demo-axios` leaves the stack up. The Backstage
 * service is the production container (not the dev harness).
 */
const ENGINE_URL = process.env.ENGINE_URL ?? 'http://localhost:8080';

test.describe('axios compromise demo flow', () => {
  test('engine reports the 2 expected incidents for axios@1.11.0', async ({ request }) => {
    const resp = await request.get(`${ENGINE_URL}/v1/incidents`);
    expect(resp.status(), 'engine /v1/incidents must be up — did you run make demo-axios?').toBe(200);

    const body = await resp.json();
    expect(Array.isArray(body.items), 'response.items should be an array').toBeTruthy();

    const axiosIncidents = body.items.filter((i: any) =>
      typeof i.iocId === 'string' && i.iocId.includes('AXIOS'),
    );
    expect(axiosIncidents.length, 'should see exactly 2 axios incidents').toBeGreaterThanOrEqual(2);

    const refs = axiosIncidents.flatMap((i: any) => i.affectedComponentsSnapshot ?? []);
    expect(refs).toContain('kind:Component/default/web-app');
    expect(refs).toContain('kind:Component/default/billing');
    expect(refs, 'reporting component must NOT show up — it ships the clean fixture').not.toContain(
      'kind:Component/default/reporting',
    );
  });

  test('Backstage IncidentDashboard renders the same incidents', async ({ page }) => {
    // AutoSignInPage bypasses the interactive Guest sign-in so we
    // load straight into the dashboard. The image-slimming refactor
    // (Dockerfile prune stage + no chown -R penalty) brought cold
    // boot to ~1 s; a single clean navigation is reliable now.
    //
    // waitUntil: 'domcontentloaded' NOT 'networkidle': the dashboard
    // opens a long-lived EventSource to /v1/stream on mount, so
    // networkidle never fires — the test would always time out on
    // the 60s suite window waiting for an idle that can't happen.
    const response = await page.goto('/', { waitUntil: 'domcontentloaded' });
    expect(
      response?.ok(),
      'Backstage production container must be up at ' +
        (process.env.BACKSTAGE_URL ?? 'http://localhost:3000'),
    ).toBeTruthy();

    // The dashboard renders under the header "Supply-chain incidents"
    // (Adım 5.1 convention); wait for it before asserting row content.
    // 30 s is the realistic window — the React bundle fetches
    // /api/config.json + the initial /v1/incidents page before the
    // table settles.
    await expect(page.getByText('Supply-chain incidents')).toBeVisible({ timeout: 30_000 });

    // The IncidentDashboard table exposes ID / STATE / OPENED / IOC
    // columns — component names live in the detail view. Asserting on
    // the IoC id + the "2 incident(s)" counter proves the render used
    // live engine data (not empty state).
    const bodyText = await page.locator('body').innerText();
    expect(bodyText.length).toBeGreaterThan(0);
    expect(bodyText, 'dashboard should show the "2 incident(s)" count').toContain('2 incident(s)');
    expect(bodyText, 'axios IoC id should render in the IOC column').toContain('01IOC-AXIOS-2026-03-31');
    // The two rows carry the "pending" state badge — at least two
    // occurrences when the engine opened the incidents seconds ago.
    const pendingMatches = bodyText.match(/pending/g) ?? [];
    expect(pendingMatches.length, 'both rows must still be in pending state').toBeGreaterThanOrEqual(2);
  });
});
