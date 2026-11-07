// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds a few bases):
//
//   DATA_DIR=/tmp/tables-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Aleksandar PORT=18252 ./pocket-tables
//   CDP=http://127.0.0.1:9342 BASE=http://127.0.0.1:18252 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18252'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)
const tableID = async (page, name) => (await api(page, '/bases')).flatMap(b => b.tables).find(t => t.name === name).id
const viewID = async (page, tid, type) => (await api(page, `/tables/${tid}`)).views.find(v => v.type === type).id

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 1500)
  assert.ok((await page.content()).includes('Your bases'), 'Home lists the bases')
  await shot(page, 'desktop-home')
  const tasks = await tableID(page, 'Tasks')
  await go(page, `/t/${tasks}`, 1500)
  assert.ok((await page.content()).includes('Write the texts'), 'the grid shows the rows')
  await shot(page, 'desktop-grid')
  const open = (await api(page, `/tables/${tasks}`)).views.find(v => v.name === 'Open').id
  await go(page, `/t/${tasks}?view=${open}`, 1500)
  await shot(page, 'desktop-grouped')
  const projects = await tableID(page, 'Projects')
  await go(page, `/t/${projects}?view=${await viewID(page, projects, 'board')}`, 1500)
  assert.ok((await page.content()).includes('New website'), 'the board shows the cards')
  await shot(page, 'desktop-board')
  const bookings = await tableID(page, 'Bookings')
  await go(page, `/t/${bookings}?view=${await viewID(page, bookings, 'calendar')}`, 1500)
  await shot(page, 'desktop-calendar')
  const coll = await tableID(page, 'Collection')
  await go(page, `/t/${coll}?view=${await viewID(page, coll, 'gallery')}`, 1500)
  await shot(page, 'desktop-gallery')
  const row = (await api(page, `/tables/${projects}/records`)).records[0].id
  await go(page, `/t/${projects}?row=${row}`, 1800)
  assert.ok((await page.content()).includes('Comments and changes'), 'a row opens with its story')
  await shot(page, 'desktop-row')
  // a cell: pick, type, Enter — then put it back
  await go(page, `/t/${tasks}`, 1200)
  await page.locator('[data-cell="0:0"]').click()
  await page.keyboard.type('Write the new texts')
  await page.keyboard.press('Enter')
  await page.waitForTimeout(800)
  const first = (await api(page, `/tables/${tasks}/records`)).records[0]
  assert.equal(first.title, 'Write the new texts', 'typing in a cell saves it')
  await page.evaluate(([id]) => fetch('/api/v1/records/' + id, { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ values: { Task: 'Write the texts' } }) }), [first.id])
  // the new-base chooser
  await go(page, '/', 800)
  await page.getByRole('button', { name: /New base/ }).first().click()
  await page.waitForTimeout(500)
  await page.getByRole('button', { name: /Ready-made/ }).click()
  await page.waitForTimeout(900)
  await shot(page, 'desktop-templates')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  const offers = await tableID(page, 'Offers')
  await go(page, `/t/${offers}`, 1500)
  await shot(page, 'desktop-dark')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1200)
  await shot(page, 'phone-home')
  const tasks = await tableID(page, 'Tasks')
  await go(page, `/t/${tasks}`, 1200)
  assert.ok((await page.content()).includes('rcard'), 'a phone shows cards')
  await shot(page, 'phone-table')
  const row = (await api(page, `/tables/${tasks}/records`)).records[0].id
  await go(page, `/t/${tasks}?row=${row}`, 1500)
  await shot(page, 'phone-row')
  const projects = await tableID(page, 'Projects')
  await go(page, `/t/${projects}?view=${await viewID(page, projects, 'board')}`, 1200)
  await shot(page, 'phone-board')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
