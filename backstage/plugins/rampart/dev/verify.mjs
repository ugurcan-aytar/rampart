#!/usr/bin/env node
// Headless Chrome verification for the rampart dev harness.
//
// Why this exists: "compile green + HTML shell curl" isn't proof that
// the React app actually mounts. This script drives system Chrome via
// puppeteer-core, loads http://localhost:3000/, captures every console
// message + uncaught exception + page error, waits for the sidebar
// route to render the plugin, then prints a verdict. Exits non-zero if
// the page throws or the expected content never appears.
//
// Usage:
//   yarn workspace @ugurcan-aytar/backstage-plugin-rampart run dev &
//   node dev/verify.mjs

import puppeteer from 'puppeteer-core';

process.on('uncaughtException', e => {
  process.stderr.write(`UNCAUGHT: ${e?.stack ?? e}\n`);
});
process.on('unhandledRejection', e => {
  process.stderr.write(`UNHANDLED: ${e?.stack ?? e}\n`);
});

const URL = process.env.RAMPART_DEV_URL ?? 'http://localhost:3000/';
const CHROME =
  process.env.PUPPETEER_EXECUTABLE_PATH ??
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';

const waitForExpect = async (fn, { timeoutMs = 10_000, intervalMs = 200 } = {}) => {
  const start = Date.now();
  let lastErr;
  while (Date.now() - start < timeoutMs) {
    try {
      const out = await fn();
      if (out !== undefined && out !== null && out !== false) return out;
    } catch (e) {
      lastErr = e;
    }
    await new Promise(r => setTimeout(r, intervalMs));
  }
  throw lastErr ?? new Error(`waitForExpect timed out after ${timeoutMs}ms`);
};

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: 'new',
  args: ['--no-sandbox'],
});

try {
  const page = await browser.newPage();
  const consoleErrors = [];
  const pageErrors = [];

  page.on('console', msg => {
    if (msg.type() === 'error') consoleErrors.push(msg.text());
  });
  page.on('pageerror', err => {
    pageErrors.push(`${err.name}: ${err.message}`);
  });

  process.stderr.write(`→ navigating to ${URL}\n`);
  await page.goto(URL, { waitUntil: 'networkidle2', timeout: 30_000 });
  process.stderr.write(`→ page.goto returned (url=${page.url()})\n`);

  await waitForExpect(async () => {
    const text = await page.evaluate(() => document.body.innerText);
    return text && text.length > 0;
  }, { timeoutMs: 15_000 });

  // NOTE: we intentionally do NOT click the Guest ENTER button.
  // Whichever mechanism we've used (page.evaluate enter.click(), native
  // puppeteer button.click(), keyboard press), puppeteer's page session
  // enters a state where evaluate / title / $$eval all hang after the
  // click — Backstage's sign-in-then-SPA-mount sequence evidently
  // tears down and rebuilds the execution context in a way
  // puppeteer-core 23 doesn't track cleanly.
  //
  // What the sign-in screen proves is already substantial:
  //   - app-config.yaml is loaded (title reads from configApi.getString)
  //   - createApp bootstrapped cleanly (no error boundary)
  //   - rampartPlugin + apis registered (plugin.test.ts covers this
  //     separately)
  //   - Zero real page errors
  //
  // Clicking through to the full IncidentDashboard is left to a real
  // browser (see dev/README.md) or Playwright — dropped here.
  process.stderr.write(`→ assertions phase (url=${page.url()})\n`);

  let title = '(not captured)';
  const finalUrl = page.url();
  let bodyText = '';
  let headings = [];
  const raceWith = (p, ms, label) =>
    Promise.race([
      p,
      new Promise((_, rej) => setTimeout(() => rej(new Error(`${label} timeout after ${ms}ms`)), ms)),
    ]);
  try {
    title = await raceWith(page.title(), 5000, 'title');
  } catch (e) {
    process.stderr.write(`  title failed: ${e?.message ?? e}\n`);
  }
  try {
    bodyText = await raceWith(page.evaluate(() => document.body.innerText), 5000, 'body');
  } catch (e) {
    process.stderr.write(`  body evaluate failed: ${e?.message ?? e}\n`);
  }
  try {
    headings = await raceWith(
      page.$$eval('h1, h2, h3, h4', nodes => nodes.map(n => n.textContent?.trim()).filter(Boolean)),
      5000,
      'headings',
    );
  } catch (e) {
    process.stderr.write(`  headings eval failed: ${e?.message ?? e}\n`);
  }

  // Filter out deprecation warnings — findDOMNode / React legacy
  // warnings are from Material-UI v4 inside Backstage's own shell
  // (not our code) and don't break the render.
  const noiseRe = /findDOMNode is deprecated|UNSAFE_|componentWillMount|componentWillReceiveProps|Support for defaultProps will be removed/;
  const realConsoleErrors = consoleErrors.filter(e => !noiseRe.test(e));
  const realPageErrors = pageErrors.filter(e => !noiseRe.test(e));

  process.stdout.write(`url:          ${finalUrl}\n`);
  process.stdout.write(`title:        ${title}\n`);
  process.stdout.write(`headings:     ${JSON.stringify(headings)}\n`);
  process.stdout.write(`body (first 500 chars):\n${bodyText.slice(0, 500)}\n\n`);
  process.stdout.write(`console errors (real): ${realConsoleErrors.length}\n`);
  for (const line of realConsoleErrors) {
    process.stdout.write(`  ✗ ${line.slice(0, 300)}\n`);
  }
  process.stdout.write(`page errors (real): ${realPageErrors.length}\n`);
  for (const line of realPageErrors) {
    process.stdout.write(`  ✗ ${line}\n`);
  }
  if (consoleErrors.length - realConsoleErrors.length > 0) {
    process.stdout.write(
      `(filtered ${consoleErrors.length - realConsoleErrors.length} MUI-v4 deprecation warnings from shell, not our code)\n`,
    );
  }

  const lowered = bodyText.toLowerCase();
  // Success signal: IncidentDashboard's "Supply-chain incidents"
  // header + empty-table markers. If these appear, React hydrated and
  // the plugin's routable extension rendered end-to-end.
  const dashboardRendered =
    lowered.includes('supply-chain incidents') &&
    (lowered.includes('no records to display') || lowered.includes('incident(s)')) &&
    headings.some(h => h && /supply-chain incidents/i.test(h));

  // ERR_CONNECTION_REFUSED fetches (engine not running in dev harness)
  // are expected — filter out.
  const allowableErrors = /ERR_CONNECTION_REFUSED|Failed to fetch|Failed to load resource/;
  const fatalConsoleErrors = realConsoleErrors.filter(e => !allowableErrors.test(e));

  if (realPageErrors.length > 0 || fatalConsoleErrors.length > 0) {
    process.exitCode = 1;
    process.stderr.write('\nverdict: runtime errors detected ✗\n');
  } else if (!dashboardRendered) {
    process.exitCode = 1;
    process.stderr.write('\nverdict: IncidentDashboard did not render ✗\n');
  } else {
    process.stderr.write('\nverdict: IncidentDashboard rendered end-to-end ✓\n');
    process.stderr.write('         • "Supply-chain incidents" heading visible\n');
    process.stderr.write('         • Empty-state table ("No records to display") mounted\n');
    process.stderr.write('         • Zero fatal console errors\n');
    process.stderr.write('         • ERR_CONNECTION_REFUSED to the engine is expected (engine not running)\n');
  }
} finally {
  await browser.close();
}
