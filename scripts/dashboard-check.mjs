// Headless dashboard check for a running Full instance with representative data.
//   PLAYWRIGHT_DIR=/dir/with/node_modules BASE=http://127.0.0.1:7319 TOKEN_FILE=agent_token node scripts/dashboard-check.mjs
// Uses a disposable instance only; reads data through the dashboard's own token prompt.
import { createRequire } from 'node:module';
import { readFileSync } from 'node:fs';
const require = createRequire(process.env.PLAYWRIGHT_DIR + '/');
const { chromium } = require('playwright');
const base = process.env.BASE, token = readFileSync(process.env.TOKEN_FILE, 'utf8').trim();
if (!/^http:\/\/(127\.0\.0\.1|localhost):\d+$/.test(base)) throw new Error('loopback only');
const want = ['task', 'decision', 'evidence', 'session', 'timeline'];
const browser = await chromium.launch();
let failed = 0;
const report = (ok, msg) => { console.log((ok ? 'PASS  ' : 'FAIL  ') + msg); if (!ok) failed++; };
for (const [label, viewport, scheme] of [['desktop light', { width: 1280, height: 900 }, 'light'], ['desktop dark', { width: 1280, height: 900 }, 'dark'], ['mobile 390 light', { width: 390, height: 844 }, 'light'], ['mobile 390 dark', { width: 390, height: 844 }, 'dark']]) {
  const ctx = await browser.newContext({ viewport, colorScheme: scheme });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', e => errors.push('pageerror: ' + e.message));
  page.on('console', m => { if (m.type() === 'error') errors.push('console: ' + m.text()); });
  page.on('requestfailed', r => errors.push('requestfailed: ' + r.url()));
  await page.goto(base + '/');
  await page.fill('#token', token);
  await page.keyboard.press('Enter');
  await page.waitForSelector('a[href^="#/p/"]', { timeout: 15000 });
  const projects = await page.locator('a[href^="#/p/"]').count();
  await page.locator('a[href^="#/p/"]').first().click();
  await page.waitForTimeout(1500);
  const text = (await page.locator('main').innerText()).toLowerCase();
  const missing = want.filter(w => !text.includes(w));
  const bg = await page.evaluate(() => getComputedStyle(document.body).backgroundColor);
  const lum = bg.match(/\d+/g).slice(0, 3).map(Number).reduce((a, b) => a + b, 0) / 3;
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  await page.goto(base + '/#/search?q=retrieved');
  await page.waitForTimeout(1500);
  const hits = (await page.locator('main').innerText()).toLowerCase().includes('retrieved');
  report(projects > 0, `${label}: overview lists ${projects} project(s)`);
  report(missing.length === 0, `${label}: project view shows ${want.join(', ')}${missing.length ? ' (missing: ' + missing + ')' : ''}`);
  report(hits, `${label}: search finds the seeded text`);
  report(scheme === 'dark' ? lum < 100 : lum > 150, `${label}: ${scheme} theme applied (background ${bg})`);
  report(overflow <= 0, `${label}: no horizontal overflow (${overflow}px)`);
  report(errors.length === 0, `${label}: no JavaScript or network errors${errors.length ? ' ' + JSON.stringify(errors.slice(0, 3)) : ''}`);
  await ctx.close();
}
await browser.close();
console.log(failed ? failed + ' failed' : 'all dashboard checks passed');
process.exit(failed ? 1 : 0);
