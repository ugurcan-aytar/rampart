import { expect, test } from '@playwright/test';
import { execSync } from 'node:child_process';

/**
 * native-fallback.spec.ts — checks ADR-0005's "opt-in hardening never
 * blocks the engine" promise on a live compose stack.
 *
 * The engine is brought up in its default profile (strategy=go, no
 * rampart-native container) but with RAMPART_PARSER_STRATEGY=native
 * so the boot path probes the sidecar socket, fails fast, and logs
 * the fallback. We grep the container's log for the exact warn line
 * and the follow-up "effective=go" resolution.
 *
 * Why not Playwright's browser? This is an engine-boot assertion
 * that has no UI surface. The spec lives here anyway because the
 * e2e/ directory is where we keep the "does the stack wire up
 * correctly" scenarios; Playwright's test runner is the cheapest
 * way to run it alongside axios-flow.
 */
test('engine falls back to go parser when strategy=native but sidecar missing', () => {
  const run = (cmd: string, opts: { cwd?: string } = {}) =>
    execSync(cmd, {
      cwd: opts.cwd ?? repoRoot(),
      stdio: ['ignore', 'pipe', 'pipe'],
      env: { ...process.env, RAMPART_PARSER_STRATEGY: 'native' },
    }).toString();

  // Clean any prior state, then bring engine up in default profile
  // with strategy=native requested — intentionally NOT launching
  // rampart-native so the probe fails.
  run('docker compose --profile native --profile full down -v --remove-orphans || true');
  run('docker compose up -d engine mock-npm-registry');

  // Give the engine a second to flush boot logs, then assert the
  // fallback warn line shows up.
  execSync('sleep 2');
  const logs = run('docker compose logs engine');

  expect(logs).toContain('rampart-native unreachable; falling back to embedded Go parser');
  expect(logs).toContain('"requested":"native"');
  expect(logs).toContain('"effective":"go"');

  // The engine must still be answering HTTP — opt-in hardening never
  // blocks the engine from serving.
  const health = run('curl -sSf http://localhost:8080/healthz');
  expect(health).toContain('"status":"ok"');

  // Tear back down so the test leaves a clean slate for the next run.
  run('docker compose --profile native --profile full down -v --remove-orphans');
});

function repoRoot(): string {
  return execSync('git rev-parse --show-toplevel').toString().trim();
}
