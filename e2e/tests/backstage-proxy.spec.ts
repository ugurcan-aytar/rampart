import { expect, test } from '@playwright/test';

/**
 * backstage-proxy.spec.ts — exercises the rampart-backend plugin's
 * /api/rampart/v1/* proxy path through the live Backstage backend.
 *
 * Why this test exists: PR #25 (express 4 → 5) shipped with green CI
 * because the existing axios-flow test only hits the engine directly
 * on :8080. The Backstage proxy path was never exercised, so a
 * `path-to-regexp` v8 incompatibility (`/v1/*` rejected at router
 * registration) crashed the rampart-backend plugin at startup
 * unnoticed. Backstage's `/` kept returning 200 from the static
 * frontend even with the backend plugin dead.
 *
 * LEVEL 1 (mount smoke) is the regression guard: any non-404 / non-5xx
 * response from /api/rampart/v1/* proves the route is registered.
 * LEVEL 2 (proxy equivalence) further proves the proxy reaches the
 * engine and returns the same payload.
 */
const BACKSTAGE_URL = process.env.BACKSTAGE_URL ?? 'http://localhost:3000';
const ENGINE_URL = process.env.ENGINE_URL ?? 'http://localhost:8080';

test.describe('rampart-backend proxy', () => {
  test('plugin mounts — /api/rampart/v1/* responds (not 404 / 5xx)', async ({ request }) => {
    // Hit the proxy without auth. Backstage's auth middleware will
    // return 401 — that's fine; 401 PROVES the route is registered
    // and reachable. A 404 would mean the rampart-backend plugin
    // failed to mount (the PR #25 regression). A 5xx would mean a
    // crash inside the proxy handler.
    const res = await request.get(`${BACKSTAGE_URL}/api/rampart/v1/incidents`);
    expect(
      [200, 401, 403],
      `proxy must mount — 404 means rampart-backend plugin crashed at startup. got ${res.status()}`,
    ).toContain(res.status());
  });

  test('proxy reaches the engine — output equals direct engine call', async ({ request }) => {
    // Backstage demo uses guest auth (app-config.yaml: providers.guest: {}).
    // Acquire a guest token via the standard backend handshake then
    // hit the proxy with it. If the proxy is end-to-end functional,
    // the JSON it returns matches what the engine returns directly.
    const tokenRes = await request.post(
      `${BACKSTAGE_URL}/api/auth/guest/refresh`,
      { data: {} },
    );

    if (tokenRes.status() !== 200) {
      // Some Backstage versions expose a different guest endpoint or
      // require a session cookie that fetch() can't easily mint. The
      // mount-smoke check above is still the load-bearing assertion;
      // skip equivalence rather than fail noisily.
      test.skip(true, `guest auth handshake returned ${tokenRes.status()} — skipping equivalence check`);
      return;
    }

    const tokenBody: { backstageIdentity?: { token?: string } } = await tokenRes.json();
    const token = tokenBody.backstageIdentity?.token;
    if (!token) {
      test.skip(true, 'guest handshake response did not include backstageIdentity.token');
      return;
    }

    const proxyRes = await request.get(
      `${BACKSTAGE_URL}/api/rampart/v1/incidents`,
      { headers: { Authorization: `Bearer ${token}` } },
    );
    expect(proxyRes.status(), 'proxy with valid guest token should return 200').toBe(200);

    const engineRes = await request.get(`${ENGINE_URL}/v1/incidents`);
    expect(engineRes.status(), 'direct engine call should also return 200').toBe(200);

    const proxyJson = await proxyRes.json();
    const engineJson = await engineRes.json();

    // Proxy payloads should be byte-identical to direct engine calls
    // (the proxy is a pass-through; no transformation). Compare the
    // incident counts + IoC ids rather than the whole object to keep
    // the diff readable when the test does fail.
    expect(proxyJson.items.length, 'proxy and engine must agree on incident count').toBe(
      engineJson.items.length,
    );
    const proxyIocs = new Set<string>(proxyJson.items.map((i: { iocId: string }) => i.iocId));
    const engineIocs = new Set<string>(engineJson.items.map((i: { iocId: string }) => i.iocId));
    expect(proxyIocs).toEqual(engineIocs);
  });
});
