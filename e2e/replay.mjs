import puppeteer from 'puppeteer-core';
const CHROME = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
setTimeout(() => { console.error('WATCHDOG timeout'); process.exit(2); }, 90_000);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
let failures = 0;
const check = (n, c, x = '') => { console.log(`${c ? 'PASS' : 'FAIL'}  ${n}${x ? '  [' + x + ']' : ''}`); if (!c) failures++; };
const feed = (p) => p.evaluate(() => document.querySelector('#feed-items').textContent);
const stats = async (p) => (await p.evaluate(() => document.querySelector('#stats').textContent)).replace(/\s+/g, ' ');

const browser = await puppeteer.launch({ executablePath: CHROME, headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu'] });
const tabA = await browser.newPage();
const tabB = await browser.newPage();
for (const t of [tabA, tabB]) t.on('pageerror', (e) => console.log('PAGEERROR', String(e)));
await tabA.goto('http://localhost:8080', { waitUntil: 'load' });
await tabB.goto('http://localhost:8080', { waitUntil: 'load' });

const sseUp = (p) => p.evaluate(() => !!document.body._htmx?.sse);
for (let i = 0; i < 50; i++) { if ((await sseUp(tabA)) && (await sseUp(tabB))) break; await sleep(100); }
check('both SSE connections up', (await sseUp(tabA)) && (await sseUp(tabB)));

// baseline event reaches both tabs
await tabA.evaluate(() => document.querySelector('#card-1 .btn-move').click());
await sleep(800);
check('B receives live event', (await feed(tabB)).includes('Moved'));

// --- kill tab B's stream the same way the extension does when a tab is
// --- backgrounded (pauseOnBackground) — reader.cancel() — then mutate.
await tabB.evaluate(() => document.body._htmx.sse.reader.cancel());
await sleep(50);
await tabA.evaluate(() => document.querySelector('[hx-post="/cards/2/move?dir=right"]').click()); // todo -> doing
await sleep(60);
await tabA.evaluate(() => document.querySelector('[hx-post="/cards/3/move?dir=right"]').click()); // doing -> done
await sleep(150);

check('B has not seen mutations yet (stream down)', !(await feed(tabB)).includes('Build kanban board'));

// --- extension reconnects (~500ms + jitter) sending Last-Event-ID; server replays
await sleep(3000);

const bFeed = await feed(tabB);
const count = (s) => (bFeed.match(new RegExp(s, 'g')) ?? []).length;
check('B caught up: event 1 replayed', bFeed.includes('Build kanban board'));
check('B caught up: event 2 replayed', bFeed.includes('Explore hx-swap'));
check('no duplicate deliveries', count('Build kanban board') === 1 && count('Explore hx-swap') === 1);
check('B stats reflect final state', (await stats(tabB)).includes('Todo: 0') && (await stats(tabB)).includes('Done: 2'));
check('B connection alive again', await sseUp(tabB));

// and live sync still works after recovery
await tabA.evaluate(() => { document.querySelector('.add-form input[name="title"]').value = 'After reconnect'; document.querySelector('.add-form button[type="submit"]').click(); });
await sleep(800);
check('B receives post-recovery live event', (await feed(tabB)).includes('After reconnect'));

await browser.close();
console.log(failures === 0 ? '\nALL REPLAY CHECKS PASSED' : `\n${failures} CHECK(S) FAILED`);
process.exit(failures === 0 ? 0 : 1);
