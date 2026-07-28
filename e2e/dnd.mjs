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

// --- SortableJS is wired up on every column ---
check('SortableJS loaded', await tabA.evaluate(() => typeof window.Sortable === 'function'));
check('all 3 columns sortable', await tabA.evaluate(() =>
  [...document.querySelectorAll('.cards')].every((el) => !!el._sortable)));

// --- drag helper: real mouse events, which SortableJS listens to ---
async function drag(page, fromSel, toSel) {
  const from = await (await page.$(fromSel)).boundingBox();
  const to = await (await page.$(toSel)).boundingBox();
  const sx = from.x + from.width / 2, sy = from.y + from.height / 2;
  const tx = to.x + to.width / 2, ty = to.y + to.height / 2;
  await page.mouse.move(sx, sy);
  await page.mouse.down();
  await page.mouse.move(sx + 4, sy + 4, { steps: 3 });   // pass drag threshold
  for (let i = 1; i <= 12; i++) {                        // glide to target
    await page.mouse.move(sx + (tx - sx) * i / 12, sy + (ty - sy) * i / 12, { steps: 2 });
    await sleep(25);
  }
  await sleep(100);
  await page.mouse.up();
}

// --- cross-column drag: card 1 (todo) → doing ---
await drag(tabA, '#card-1', '#col-doing .cards');
await sleep(900);
check('A: card 1 dropped into doing', await tabA.evaluate(() => !!document.querySelector('#col-doing #card-1')));
check('A: card 1 gone from todo', await tabA.evaluate(() => !document.querySelector('#col-todo #card-1')));
check('B: cross-column drop synced via SSE', await tabB.evaluate(() => !!document.querySelector('#col-doing #card-1')));
check('feed logged the move', (await tabA.evaluate(() => document.querySelector('#feed-items').textContent)).includes('Moved'));

// --- same-column reorder: drag card 3 below card 1 in doing ---
await drag(tabA, '#card-3', '#col-doing .cards');
await sleep(900);
const orderA = await tabA.evaluate(() =>
  [...document.querySelectorAll('#col-doing .card')].map((c) => c.id));
check('A: doing order is [card-1, card-3]', JSON.stringify(orderA) === JSON.stringify(['card-1', 'card-3']), JSON.stringify(orderA));

// --- persistence: a brand-new tab loads the server's order ---
const tabC = await browser.newPage();
await tabC.goto('http://localhost:8080', { waitUntil: 'load' });
await sleep(500);
const orderC = await tabC.evaluate(() =>
  [...document.querySelectorAll('#col-doing .card')].map((c) => c.id));
check('C: fresh load shows persisted order', JSON.stringify(orderC) === JSON.stringify(['card-1', 'card-3']), JSON.stringify(orderC));
check('C: fresh load has card 1 in doing', await tabC.evaluate(() => !!document.querySelector('#col-doing #card-1')));

// --- sortables survive htmx re-renders (columns were swapped many times) ---
check('A: columns still sortable after swaps', await tabA.evaluate(() =>
  [...document.querySelectorAll('.cards')].every((el) => !!el._sortable)));

await browser.close();
console.log(failures === 0 ? '\nALL DND CHECKS PASSED' : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
