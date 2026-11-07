// Test pages with KNOWN answers. A blocking script adds `ff` to <html> in
// Firefox, and each page's `.ff` rules plant exactly one divergence. Expected
// root causes live next to the page so page and answer can't drift apart.
//
// expect:       groups that must each be found; a group is satisfied when a
//               finding sits on an element whose id is in `anyOf`
// maxFindings:  how many findings are acceptable (tests cascade collapsing)
// onlyTags:     findings must sit on these tags (genuine-difference pages)
// kind:         'planted' (counts toward recall), 'native' or 'control'

const BASE_CSS = `
* { box-sizing: border-box; }
body { margin: 0; font: 16px/1.5 sans-serif; color: #222; }
header { display: flex; align-items: center; justify-content: space-between; padding: 16px 32px; border-bottom: 1px solid #ddd; }
header nav { display: flex; gap: 16px; }
main { max-width: 1100px; margin: 0 auto; padding: 32px; }
section { margin: 32px 0; }
.grid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px; }
.card { border: 1px solid #ccc; border-radius: 8px; padding: 20px; }
.card h3 { margin: 0 0 8px; }
.price { font-size: 32px; font-weight: bold; }
button { appearance: none; font: inherit; padding: 8px 16px; border: 1px solid #333; border-radius: 4px; background: #eee; }
footer { padding: 32px; border-top: 1px solid #ddd; text-align: center; color: #666; }
`;

const NAV = (ids = []) => `<nav id="nav">${['Home', 'Docs', 'Pricing', 'Blog', 'About']
  .map((t, i) => `<a href="#${t.toLowerCase()}"${ids[i] ? ` id="${ids[i]}"` : ''}>${t}</a>`).join('')}</nav>`;
const HEADER = (nav = NAV()) => `<header><strong>Acme</strong>${nav}</header>`;
const FOOTER = '<footer>© 2026 Acme Inc. · <a href="#privacy">Privacy</a> · <a href="#terms">Terms</a></footer>';
const LOREM = 'Omnisnap compares how each browser engine lays out the same page and reports the element where a difference starts, instead of flagging every pixel that moved as a result.';
const SECTIONS = (n) => Array.from({ length: n }, (_, i) => `<section><h2>Section ${i + 1}</h2><p>${LOREM}</p></section>`).join('');
const card = (title, price, attrs = '', extra = '') => `<div class="card"${attrs}><h3>${title}</h3><div class="price">${price}</div>${extra}<ul><li>Unlimited projects</li><li>Priority support</li><li>Custom domains</li></ul><button>Choose ${title}</button></div>`;
const rows = (n) => Array.from({ length: n }, (_, i) => `<tr><td>2026-10-${String(i + 1).padStart(2, '0')}</td><td>project-${i + 1}</td><td>${(i * 37) % 500} builds</td><td>${(i * 13) % 97}%</td></tr>`).join('');

export const PAGES = [
  {
    slug: '01-card-width', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: '.ff #target { width: 240px; }',
    body: `<main><h1>Pricing</h1><p>${LOREM}</p><section class="grid">${card('Starter', '$9')}${card('Pro', '$29', ' id="target"')}${card('Team', '$99')}</section>${SECTIONS(2)}</main>`,
  },
  {
    slug: '02-heading-wrap', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.hero h1 { font-size: 40px; max-width: 600px; margin: 0 0 16px; } .ff #target { font-size: 56px; }',
    body: `<main><section class="hero"><h1 id="target">Every engine, one command</h1><p>${LOREM}</p><button>Get started</button></section>${SECTIONS(4)}</main>`,
  },
  {
    slug: '03-overflow', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: 'table { border-collapse: collapse; width: 100%; } td { border: 1px solid #ddd; padding: 6px 10px; } .ff #target { min-width: 1600px; }',
    body: `<main><h1>Usage report</h1><p>${LOREM}</p><div class="table-wrap"><table id="target"><tbody>${rows(12)}</tbody></table></div>${SECTIONS(1)}</main>`,
  },
  {
    slug: '04-missing', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.banner { background: #fffae6; padding: 12px 32px; border-bottom: 1px solid #eedc82; } .ff #target { display: none; }',
    body: `<div class="banner" id="target">Scheduled maintenance on Saturday, 02:00 to 04:00 UTC.</div><main><h1>Dashboard</h1><p>${LOREM}</p>${SECTIONS(3)}</main>`,
  },
  {
    slug: '05-padding', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: '.ff #target { padding-top: 48px; }',
    body: `<main><h1>Plans</h1><section class="grid">${card('Starter', '$9')}${card('Pro', '$29', ' id="target"')}${card('Team', '$99')}</section>${SECTIONS(2)}</main>`,
  },
  {
    slug: '06-order', kind: 'planted', expect: [{ anyOf: ['a', 'b', 'nav'] }], maxFindings: 2,
    header: HEADER(NAV([null, 'a', 'b'])),
    css: '#nav a:nth-child(1){order:1} #nav a:nth-child(2){order:2} #nav a:nth-child(3){order:3} #nav a:nth-child(4){order:4} #nav a:nth-child(5){order:5} .ff #nav #a{order:3} .ff #nav #b{order:2}',
    body: `<main><h1>Navigation</h1><p>${LOREM}</p>${SECTIONS(2)}</main>`,
  },
  {
    slug: '07-font', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.ff #target { font-family: monospace; }',
    body: `<main><h1>Changelog</h1><p id="target">${LOREM} ${LOREM}</p><p>${LOREM}</p>${SECTIONS(2)}</main>`,
  },
  {
    slug: '08-forms', kind: 'native', expect: [], onlyTags: ['input', 'select', 'textarea', 'button', 'option'], maxFindings: 15,
    css: 'form { display: grid; grid-template-columns: 200px 1fr; gap: 12px 16px; max-width: 700px; } label { align-self: center; } form button { appearance: auto; background: revert; border: revert; padding: revert; }',
    body: `<main><h1>Native form controls</h1><p>No planted difference: whatever differs here is the engines' own default rendering.</p><form>
      <label for="f1">Name</label><input id="f1" type="text" value="Ada Lovelace">
      <label for="f2">Email</label><input id="f2" type="email" placeholder="you@example.com">
      <label for="f3">Plan</label><select id="f3"><option>Starter</option><option>Pro</option><option>Team</option></select>
      <label for="f4">Seats</label><input id="f4" type="number" value="3">
      <label for="f5">Start date</label><input id="f5" type="date" value="2026-11-06">
      <label for="f6">Brand color</label><input id="f6" type="color" value="#3366ff">
      <label for="f7">Volume</label><input id="f7" type="range" min="0" max="10" value="4">
      <label for="f8">Newsletter</label><input id="f8" type="checkbox" checked>
      <label for="f9">Billing</label><span><input id="f9" type="radio" name="b" checked> Monthly <input type="radio" name="b"> Yearly</span>
      <label for="f10">Logo</label><input id="f10" type="file">
      <label for="f11">Notes</label><textarea id="f11" rows="3">Ship it.</textarea>
      <span></span><button type="button">Submit</button>
    </form></main>`,
  },
  {
    slug: '09-control-article', kind: 'control', expect: [], maxFindings: 0,
    css: 'article { max-width: 720px; } pre { background: #f6f6f6; padding: 12px; overflow: auto; } table { border-collapse: collapse; } td, th { border: 1px solid #ccc; padding: 4px 8px; } blockquote { border-left: 4px solid #ccc; margin-left: 0; padding-left: 16px; }',
    body: `<main><article><h1>A control article</h1><p>${LOREM}</p><h2>Lists</h2><ul><li>First point</li><li>Second point with <a href="#x">a link</a></li><li>Third point</li></ul><ol><li>One</li><li>Two</li></ol>
      <blockquote>${LOREM}</blockquote><pre><code>const engines = ['blink', 'gecko', 'webkit'];\nfor (const e of engines) console.log(e);</code></pre>
      <table><thead><tr><th>Engine</th><th>Protocol</th></tr></thead><tbody><tr><td>Blink</td><td>CDP</td></tr><tr><td>Gecko</td><td>BiDi</td></tr></tbody></table>
      <hr><figure><svg width="320" height="120" viewBox="0 0 320 120"><rect x="10" y="10" width="300" height="100" fill="#cde"/><circle cx="160" cy="60" r="40" fill="#79a"/></svg><figcaption>An inline SVG figure.</figcaption></figure><p>${LOREM}</p></article></main>`,
  },
  {
    slug: '10-control-layout', kind: 'control', expect: [], maxFindings: 0,
    css: '.layout { display: grid; grid-template-columns: 240px 1fr; gap: 32px; } aside { position: sticky; top: 0; align-self: start; } .cols { column-count: 2; column-gap: 32px; } .row { display: flex; justify-content: space-between; align-items: center; gap: 12px; }',
    body: `<main><div class="layout"><aside><h3>Sidebar</h3><ul><li>Overview</li><li>Settings</li><li>Billing</li></ul></aside><div><div class="row"><h1>Control layout</h1><button>New project</button></div>
      <section class="grid">${card('Starter', '$9')}${card('Pro', '$29')}${card('Team', '$99')}</section><div class="cols"><p>${LOREM}</p><p>${LOREM}</p><p>${LOREM}</p></div></div></div></main>`,
  },
  {
    slug: '11-clip', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.ff #target { max-height: 48px; overflow: hidden; }',
    body: `<main><h1>Features</h1><section class="grid">${card('Starter', '$9')}${card('Pro', '$29', '', `<p id="target">${LOREM}</p>`)}${card('Team', '$99')}</section>${SECTIONS(2)}</main>`,
  },
  {
    slug: '12-gap', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.tags { display: flex; flex-wrap: wrap; gap: 8px; } .tag { padding: 4px 10px; border-radius: 12px; background: #eef; } .ff #target { gap: 28px; }',
    body: `<main><h1>Tags</h1><p>${LOREM}</p><div class="tags" id="target">${['css', 'layout', 'flexbox', 'grid', 'firefox', 'chrome', 'safari'].map((t) => `<span class="tag">${t}</span>`).join('')}</div>${SECTIONS(2)}</main>`,
  },
  {
    slug: '13-image', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: 'figure { margin: 0; } figure svg { display: block; width: 100%; height: auto; } .ff #target { width: 60%; }',
    body: `<main><h1>Report</h1><figure><svg id="target" viewBox="0 0 400 160"><rect x="0" y="0" width="400" height="160" fill="#eef"/><rect x="20" y="60" width="60" height="80" fill="#79a"/><rect x="100" y="30" width="60" height="110" fill="#79a"/><rect x="180" y="80" width="60" height="60" fill="#79a"/></svg><figcaption>Weekly builds per project.</figcaption></figure>${SECTIONS(2)}</main>`,
  },
];

// Knowledge-base fixtures: no planted difference, but known ENGINE
// differences. Engines may differ here; every difference must be explained
// by the knowledge base and stay in the Info tier (nothing Reported).
const TERMINAL = Array.from({ length: 30 }, (_, i) => `<div>$ step ${i + 1}: compile, link, ship</div>`).join('');
PAGES.push(
  {
    slug: '14-kb-line-height', kind: 'kb', kb: 'K1',
    css: '.term { font: 14px/1.625 monospace; background: #111; color: #ddd; padding: 16px; } .copy p { font-size: 14px; line-height: 1.625; }',
    body: `<main><h1>Fractional line-height</h1><div class="term">${TERMINAL}</div><div class="copy"><p>${LOREM} ${LOREM}</p><p>${LOREM}</p></div></main>`,
  },
  {
    slug: '15-kb-line-height-normal', kind: 'kb', kb: 'K2',
    css: 'table { border-collapse: collapse; font: 13px sans-serif; line-height: normal; } td { padding: 0 6px; }',
    body: `<main><h1>line-height: normal</h1><table><tbody>${rows(40)}</tbody></table></main>`,
  },
);

export function renderPage(p) {
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>${p.slug}</title>`
    + '<script>if (navigator.userAgent.includes("Firefox")) document.documentElement.classList.add("ff");</script>'
    + `<style>${BASE_CSS}${p.css || ''}</style></head><body>${p.header ?? HEADER()}${p.body}${FOOTER}</body></html>`;
}
