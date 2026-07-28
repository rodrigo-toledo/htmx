import puppeteer from 'puppeteer-core';
const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
setTimeout(() => { console.error('WATCHDOG timeout'); process.exit(2); }, 120_000);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let failures = 0;
const check = (n, c, x = '') => { console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  [' + x + ']' : ''}`); if (!c) failures++; };

const browser = await puppeteer.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu'] });
const tabA = await browser.newPage();
const tabB = await browser.newPage();
for (const t of [tabA, tabB]) t.on('pageerror', (e) => console.log('PAGEERROR', String(e)));
await tabA.goto('http://localhost:8080', { waitUntil: 'load' });
await tabB.goto('http://localhost:8080', { waitUntil: 'load' });
await sleep(1000);

// Creating tabB backgrounded tabA, and Chrome pauses requestAnimationFrame in
// hidden tabs — which stalls Alpine's effect flush (the modal would never
// close). A real user always acts in the focused tab, so mirror that here.
// (htmx swaps in tabB still work: they don't rely on rAF, and SSE stays alive
// via pauseOnBackground:false.)
await tabA.bringToFront();
await sleep(200);

const visible = (p, sel) => p.evaluate((s) => {
  const el = document.querySelector(s);
  return !!el && getComputedStyle(el).display !== 'none';
}, sel);
const click = (p, sel) => p.evaluate((s) => document.querySelector(s).click(), sel);

check('Alpine loaded', await tabA.evaluate(() => typeof window.Alpine === 'object'));
check('modal hidden initially', !(await visible(tabA, '.modal-backdrop')));

// --- open: shows the right card title ---
await click(tabA, '#card-1 .btn-delete');
await sleep(300);
check('modal opens on delete click', await visible(tabA, '.modal-backdrop'));
check('modal shows card title', (await tabA.evaluate(() => document.querySelector('.modal-card').textContent)).includes('Learn htmx basics'));

// --- escape closes, card survives ---
await tabA.keyboard.press('Escape');
await sleep(300);
check('escape closes modal', !(await visible(tabA, '.modal-backdrop')));
check('card still present after cancel', await tabA.evaluate(() => !!document.querySelector('#card-1')));

// --- click-outside closes ---
await click(tabA, '#card-1 .btn-delete');
await sleep(300);
const bb = await (await tabA.$('.modal-backdrop')).boundingBox();
await tabA.mouse.click(bb.x + 5, bb.y + 5);  // backdrop corner, outside the modal
await sleep(300);
check('click-outside closes modal', !(await visible(tabA, '.modal-backdrop')));

// --- confirm deletes, syncs to tab B ---
await click(tabA, '#card-2 .btn-delete');
await sleep(300);
await click(tabA, '.modal-actions .danger');
await sleep(700);
check('modal closed after confirm', !(await visible(tabA, '.modal-backdrop')));
check('card deleted in A', await tabA.evaluate(() => !document.querySelector('#card-2')));
check('delete synced to B', await tabB.evaluate(() => !document.querySelector('#card-2')));
check('feed logged delete', (await tabA.evaluate(() => document.querySelector('#feed-items').textContent)).includes('Deleted'));

// --- THE integration test: a swapped-in card's @click must work ---
// create a card (htmx swaps in a fresh column), then delete it via the modal
await tabA.evaluate(() => { document.querySelector('.add-form input[name="title"]').value = 'Alpine test'; });
await click(tabA, '.add-form button[type="submit"]');
await sleep(700);
const newId = await tabA.evaluate(() =>
  [...document.querySelectorAll('#col-todo .card')].find(c => c.textContent.includes('Alpine test'))?.id);
check('new card swapped in', !!newId);
await click(tabA, `#${newId} .btn-delete`);   // @click on a node htmx just created
await sleep(300);
check('Alpine bound on swapped-in card (hx-alpine-compat)', await visible(tabA, '.modal-backdrop'));
await click(tabA, '.modal-actions .danger');
await sleep(700);
check('swapped-in card deleted', await tabA.evaluate(() =>
  ![...document.querySelectorAll('#col-todo .card')].some(c => c.textContent.includes('Alpine test'))));

await browser.close();
console.log(failures === 0 ? '\nALL MODAL CHECKS PASSED' : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
