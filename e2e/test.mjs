import puppeteer from 'puppeteer-core';

const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const BASE = 'http://localhost:8080';

setTimeout(() => { console.error('WATCHDOG: test timed out'); process.exit(2); }, 120_000);

let failures = 0;
const check = (name, cond, extra = '') => {
  console.log(`${cond ? 'PASS' : 'FAIL'}  ${name}${extra ? '  [' + extra + ']' : ''}`);
  if (!cond) failures++;
};
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const browser = await puppeteer.launch({
  executablePath: CHROME,
  headless: true,
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu'],
});

const tabA = await browser.newPage();
const tabB = await browser.newPage();

const badResources = [];
const pageErrors = { A: [], B: [] };
for (const [name, page] of [['A', tabA], ['B', tabB]]) {
  page.on('response', (res) => {
    // the 422 on PATCH /cards/{id} is the intentional validation demo
    const intentional = res.status() === 422 && /\/cards\/\d+$/.test(res.url());
    if (res.status() >= 400 && !res.url().includes('favicon') && !intentional) {
      badResources.push(`${res.status()} ${res.url()}`);
    }
  });
  page.on('pageerror', (e) => pageErrors[name].push(String(e)));
}

// evaluate-based helpers (puppeteer's native click hangs on this Chrome build)
const click = (page, sel) => page.evaluate((s) => document.querySelector(s).click(), sel);
const q = (page, sel) => page.evaluate((s) => !!document.querySelector(s), sel);
const text = (page, sel) => page.evaluate((s) => document.querySelector(s)?.textContent ?? '', sel);
const hasClass = (page, sel, cls) => page.evaluate(([s, c]) => document.querySelector(s)?.classList.contains(c), [sel, cls]);

await tabA.goto(BASE, { waitUntil: 'load' });
await tabB.goto(BASE, { waitUntil: 'load' });

// wait until both tabs hold a live SSE connection (extension stores it on the element)
const sseUp = (page) => page.evaluate(() => !!document.body._htmx?.sse);
for (let i = 0; i < 50; i++) {
  if ((await sseUp(tabA)) && (await sseUp(tabB))) break;
  await sleep(100);
}
check('A: SSE connection established', await sseUp(tabA));
check('B: SSE connection established', await sseUp(tabB));

// --- initial render ---
check('A: 3 columns rendered', await page_count(tabA, '.column') === 3);
check('A: stats bar has class', await hasClass(tabA, '#stats', 'stats'));
check('A: htmx loaded', await tabA.evaluate(() => typeof window.htmx === 'object'));

async function page_count(page, sel) {
  return page.evaluate((s) => document.querySelectorAll(s).length, sel);
}

// --- move card 1 (todo → doing) in tab A ---
await click(tabA, '#card-1 .btn-move');
await sleep(700);

check('A: card 1 now in doing', await q(tabA, '#col-doing #card-1'));
check('A: card 1 gone from todo', !(await q(tabA, '#col-todo #card-1')));
check('A: doing column kept its class', await hasClass(tabA, '#col-doing', 'column'));
check('A: todo column kept its class', await hasClass(tabA, '#col-todo', 'column'));
check('A: stats kept its class', await hasClass(tabA, '#stats', 'stats'));
check('A: stats counts updated', (await text(tabA, '#stats')).includes('Todo: 1') && (await text(tabA, '#stats')).includes('Doing: 2'));
check('A: todo count header updated', (await text(tabA, '#count-todo')) === '(1)');
check('A: doing count header updated', (await text(tabA, '#count-doing')) === '(2)');
check('A: feed item in acting tab', (await text(tabA, '#feed-items')).includes('Moved'));

// --- tab B: SSE side-channel ---
check('B: feed item appeared via SSE', (await text(tabB, '#feed-items')).includes('Moved'));
check('B: stats updated via SSE', (await text(tabB, '#stats')).includes('Doing: 2'));
check('B: stats kept class after SSE morph', await hasClass(tabB, '#stats', 'stats'));
check('B: progress bar present after morph', await q(tabB, '#stats .progress-fill'));

// --- boundary buttons: done-column card has only ◀ ---
check('A: done card has exactly 1 move btn (◀)', await tabA.evaluate(() => document.querySelectorAll('#col-done .card .btn-move').length === 1));
check('A: todo card has exactly 1 move btn (▶)', await tabA.evaluate(() => document.querySelectorAll('#col-todo .card .btn-move').length === 1));

// --- move card 1 back left (doing → todo), then right again: repeated OOB ---
await click(tabA, '#card-1 .btn-move'); // ◀ is first (and only) move btn in doing? no—doing has both ◀ and ▶
await sleep(700);

// --- inline edit ---
await click(tabA, '#card-2 .btn-edit');
await sleep(500);
check('A: edit form shown', await q(tabA, '#card-2 form'));
await tabA.evaluate(() => {
  const input = document.querySelector('#card-2 input[name="title"]');
  input.value = input.value + ' (edited)';
});
await click(tabA, '#card-2 form button[type="submit"]');
await sleep(500);
check('A: edit saved, view restored', (await text(tabA, '#card-2 .card-title')).includes('(edited)'));

// --- validation: empty title → 422 re-renders form with error ---
await click(tabA, '#card-2 .btn-edit');
await sleep(500);
await tabA.evaluate(() => { document.querySelector('#card-2 input[name="title"]').value = ''; });
await click(tabA, '#card-2 form button[type="submit"]');
await sleep(500);
check('A: 422 swapped error form', await q(tabA, '#card-2 .error'));
await click(tabA, '#card-2 form button[type="button"]'); // cancel
await sleep(400);
check('A: cancel restored view', await q(tabA, '#card-2 .card-title'));

// --- create: count updates ---
await tabA.evaluate(() => { document.querySelector('.add-form input[name="title"]').value = 'Fresh card'; });
await click(tabA, '.add-form button[type="submit"]');
await sleep(500);
check('A: new card appended', (await text(tabA, '#col-todo .cards')).includes('Fresh card'));
check('A: todo count updated on create', (await text(tabA, '#count-todo')) === '(3)');
check('B: create arrived via SSE', (await text(tabB, '#feed-items')).includes('Fresh card'));

// --- delete: confirm dialog + count updates ---
tabA.once('dialog', (d) => d.accept());
await click(tabA, '#col-todo .card:last-child .btn-delete');
await sleep(500);
check('A: card removed', !(await text(tabA, '#col-todo .cards')).includes('Fresh card'));
check('A: todo count updated on delete', (await text(tabA, '#count-todo')) === '(2)');

// --- search (debounced) ---
await tabA.evaluate(() => {
  const input = document.querySelector('.toolbar input[type="search"]');
  input.value = 'htmx';
  input.dispatchEvent(new Event('input', { bubbles: true }));
});
await sleep(900);
check('A: search filters board', (await text(tabA, '#board')).includes('Learn htmx') && !(await text(tabA, '#board')).includes('Setup Go project'));

// --- hygiene ---
check('no failed resources (4xx/5xx)', badResources.length === 0, badResources.join(' | '));
check('no JS page errors', pageErrors.A.length + pageErrors.B.length === 0, [...pageErrors.A, ...pageErrors.B].join(' | '));

await browser.close();
console.log(failures === 0 ? '\nALL CHECKS PASSED' : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
