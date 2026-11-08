// Test pages with KNOWN answers. A blocking script adds `ff` to <html> in
// Firefox, and each page's `.ff` rules plant exactly one divergence. Expected
// root causes live next to the page so page and answer can't drift apart.
//
// expect:       groups that must each be found; a group is satisfied when a
//               finding sits on an element whose id is in `anyOf`
// maxFindings:  how many findings are acceptable (tests cascade collapsing)
// onlyTags:     findings must sit on these tags (genuine-difference pages)
// kind:         'planted' (counts toward recall), 'native', 'control' or 'kb'
// intent:       the planted bug breaks a design relation (also scored by intent)

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

// Round 5: realistic cross-engine bug patterns. Each `.ff` rule imitates a
// bug class developers hit between engines (missing feature fallback,
// different default, different sizing rule). `intent: true` marks pages
// whose bug breaks a relation the design clearly keeps (scored separately
// by lib/intent.mjs as well as by recall).
const PCARD = (title, body, attrs = '', btn = '') => `<div class="pcard"${attrs}><h3>${title}</h3><p>${body}</p><button class="cta"${btn}>Choose ${title}</button></div>`;
const PCARD_CSS = '.pgrid { display: grid; grid-template-columns: repeat(3, 1fr); gap: 24px; align-items: stretch; } .pcard { display: flex; flex-direction: column; border: 1px solid #ccc; border-radius: 8px; padding: 20px; } .pcard h3 { margin: 0 0 8px; } .pcard .cta { margin-top: auto; }';
const SHORT = 'Everything in Starter.';
const LONG = `${LOREM} Plus audit logs, single sign-on and a dedicated support engineer.`;
PAGES.push(
  {
    // margin-top: auto ignored: the CTA no longer sits on the shared bottom line
    slug: '16-card-cta-align', kind: 'planted', intent: true, expect: [{ anyOf: ['target', 'target-card'] }], maxFindings: 2,
    css: `${PCARD_CSS} .ff #target { margin-top: 0; }`,
    body: `<main><h1>Plans</h1><section class="pgrid">${PCARD('Starter', SHORT)}${PCARD('Pro', LONG)}${PCARD('Team', SHORT, ' id="target-card"', ' id="target"')}</section>${SECTIONS(2)}</main>`,
  },
  {
    // one value cell indented: the definition list's value column breaks
    slug: '17-grid-column', kind: 'planted', intent: true, expect: [{ anyOf: ['target', 'target-row'] }], maxFindings: 2,
    css: '.props { max-width: 640px; } .prow { display: grid; grid-template-columns: 180px 1fr; padding: 6px 0; border-bottom: 1px solid #eee; } .ff #target { padding-left: 12px; }',
    body: `<main><h1>Project settings</h1><div class="props">${['Name', 'Region', 'Runtime', 'Plan', 'Owner', 'Created'].map((k, i) => `<div class="prow"${i === 3 ? ' id="target-row"' : ''}><span class="k">${k}</span><span class="v"${i === 3 ? ' id="target"' : ''}>${['omnisnap', 'eu-west-1', 'go 1.25', 'Pro', 'German', '2026-11-07'][i]}</span></div>`).join('')}</div>${SECTIONS(2)}</main>`,
  },
  {
    // one toolbar button 4px lower (vertical-align/baseline difference)
    slug: '18-toolbar-align', kind: 'planted', intent: true, expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.toolbar { display: flex; gap: 8px; align-items: center; padding: 8px; border: 1px solid #ddd; border-radius: 6px; } .toolbar button { padding: 6px 12px; } .ff #target { position: relative; top: 4px; }',
    body: `<main><h1>Editor</h1><div class="toolbar">${['Bold', 'Italic', 'Link', 'Quote', 'Code', 'Image'].map((t, i) => `<button${i === 3 ? ' id="target"' : ''}>${t}</button>`).join('')}</div><p>${LOREM}</p>${SECTIONS(2)}</main>`,
  },
  {
    // extra margin on one item: the list's even rhythm breaks
    slug: '19-list-rhythm', kind: 'planted', intent: true, expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.features { display: flex; flex-direction: column; gap: 16px; max-width: 560px; } .feature { padding: 12px 16px; background: #f5f7fa; border-radius: 6px; } .ff #target { margin-top: 14px; }',
    body: `<main><h1>Features</h1><div class="features">${['Every engine', 'One command', 'Root causes', 'Fix hints', 'CI reports', 'Zero config'].map((t, i) => `<div class="feature"${i === 3 ? ' id="target"' : ''}><b>${t}</b></div>`).join('')}</div>${SECTIONS(2)}</main>`,
  },
  {
    // align-self differs: one stat tile stops stretching to the row height
    slug: '20-equal-height-tiles', kind: 'planted', intent: true, expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.tiles { display: flex; gap: 16px; align-items: stretch; } .tile { flex: 1; padding: 16px; border: 1px solid #ccc; border-radius: 8px; } .tile .n { font-size: 28px; font-weight: bold; } .ff #target { align-self: flex-start; }',
    body: `<main><h1>Overview</h1><div class="tiles"><div class="tile"><div class="n">128</div><div>builds this week, across every project and engine</div></div><div class="tile" id="target"><div class="n">97%</div><div>pass</div></div><div class="tile"><div class="n">4</div><div>engines</div></div></div>${SECTIONS(2)}</main>`,
  },
  {
    // negative margin: the status badge overlaps the title
    slug: '21-badge-overlap', kind: 'planted', intent: true, expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.head { display: flex; align-items: center; gap: 12px; } .head h2 { margin: 0; } .badge { padding: 2px 8px; border-radius: 10px; background: #e6f4ea; color: #137333; font-size: 13px; } .ff #target { margin-left: -48px; }',
    body: `<main><div class="head"><h2>Deployment 42</h2><span class="badge" id="target">passing</span></div><p>${LOREM}</p>${SECTIONS(2)}</main>`,
  },
  {
    // text-overflow unsupported: clipped without the ellipsis
    slug: '22-ellipsis', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.files { max-width: 320px; } .file { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; padding: 4px 0; } .ff #target { text-overflow: clip; }',
    body: `<main><h1>Files</h1><div class="files"><div class="file">README.md</div><div class="file" id="target">spikes/layout-diff/lib/a-very-long-module-name-that-does-not-fit.mjs</div><div class="file">go.mod</div></div>${SECTIONS(2)}</main>`,
  },
  {
    // line-clamp unsupported: the full description shows, the card grows
    slug: '23-line-clamp', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: '.posts { display: grid; grid-template-columns: repeat(3, 1fr); gap: 20px; } .post p { display: -webkit-box; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; } .ff #target { display: block; -webkit-line-clamp: unset; overflow: visible; }',
    body: `<main><h1>Blog</h1><div class="posts">${['Release 1.0', 'Why layout diffs', 'Engines compared'].map((t, i) => `<article class="post"><h3>${t}</h3><p${i === 1 ? ' id="target"' : ''}>${LOREM} ${LOREM}</p></article>`).join('')}</div>${SECTIONS(2)}</main>`,
  },
  {
    // aspect-ratio unsupported: the media placeholder collapses to a fixed height
    slug: '24-aspect-ratio', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: '.media { aspect-ratio: 16 / 9; background: #dde3ea; border-radius: 8px; } .ff #target { aspect-ratio: auto; height: 120px; }',
    body: `<main><h1>Demo</h1><div class="media" id="target"></div><p>${LOREM}</p>${SECTIONS(2)}</main>`,
  },
  {
    // narrower header: the navigation wraps onto a second line
    slug: '25-nav-wrap', kind: 'planted', expect: [{ anyOf: ['target', 'nav'] }], maxFindings: 3,
    header: `<header><strong>Acme</strong><nav id="nav" class="wide">${['Home', 'Docs', 'Pricing', 'Blog', 'About', 'Careers', 'Status'].map((t) => `<a href="#${t.toLowerCase()}">${t}</a>`).join('')}</nav></header>`,
    css: 'header nav.wide { flex-wrap: wrap; } .ff #nav { max-width: 260px; }',
    body: `<main><h1>Home</h1><p>${LOREM}</p>${SECTIONS(3)}</main>`,
  },
  {
    // white-space differs: the code block scrolls instead of wrapping
    slug: '26-code-scroll', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: 'pre { background: #111; color: #ddd; padding: 12px; white-space: pre-wrap; overflow-x: auto; max-width: 640px; } .ff #target { white-space: pre; }',
    body: `<main><h1>Install</h1><pre id="target">curl -fsSL https://omnisnap.org/install.sh | sh -s -- --engines chrome,firefox,webkit --report out/report.html --fail-on report</pre>${SECTIONS(2)}</main>`,
  },
  {
    // visibility differs: the notice is invisible but still takes space
    slug: '27-hidden-notice', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 2,
    css: '.notice { padding: 12px 16px; background: #fff4e5; border: 1px solid #f0c36d; border-radius: 6px; } .ff #target { visibility: hidden; }',
    body: `<main><h1>Billing</h1><div class="notice" id="target">Your trial ends in 3 days.</div><p>${LOREM}</p>${SECTIONS(2)}</main>`,
  },
  {
    // table-layout differs: column widths are distributed differently
    slug: '28-table-layout', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: '#target { width: 100%; border-collapse: collapse; } #target td { border: 1px solid #ddd; padding: 6px 10px; } .ff #target { table-layout: fixed; }',
    body: `<main><h1>Builds</h1><table id="target"><tbody>${rows(8)}</tbody></table>${SECTIONS(1)}</main>`,
  },
  {
    // min-width: auto vs 0 on a flex item: a long URL pushes the row wider
    slug: '29-flex-min-width', kind: 'planted', expect: [{ anyOf: ['target'] }], maxFindings: 3,
    css: '.link-row { display: flex; gap: 12px; max-width: 520px; border: 1px solid #ddd; padding: 8px; } .link-row .url { min-width: 0; overflow-wrap: anywhere; } .ff #target { min-width: auto; overflow-wrap: normal; }',
    body: `<main><h1>Webhooks</h1><div class="link-row"><b>Endpoint</b><span class="url" id="target">https://hooks.example.com/omnisnap?secret=aVeryLongWebhookSecretTokenWithoutAnyBreakOpportunity0123456789abcdefABCDEF</span></div>${SECTIONS(2)}</main>`,
  },
  {
    // control: equal-height cards, pinned CTAs, even rhythm; nothing planted
    slug: '30-control-cards', kind: 'control',
    css: `${PCARD_CSS} .features { display: flex; flex-direction: column; gap: 16px; } .feature { padding: 12px; background: #f5f7fa; }`,
    body: `<main><h1>Plans</h1><section class="pgrid">${PCARD('Starter', SHORT)}${PCARD('Pro', LONG)}${PCARD('Team', SHORT)}</section><div class="features">${['One', 'Two', 'Three', 'Four'].map((t) => `<div class="feature">${t}</div>`).join('')}</div>${SECTIONS(2)}</main>`,
  },
  {
    // control: dashboard with tiles, toolbar, settings grid and table
    slug: '31-control-dashboard', kind: 'control',
    css: '.tiles { display: flex; gap: 16px; } .tile { flex: 1; padding: 16px; border: 1px solid #ccc; } .toolbar { display: flex; gap: 8px; align-items: center; } .prow { display: grid; grid-template-columns: 180px 1fr; padding: 6px 0; } table { width: 100%; border-collapse: collapse; } td { border: 1px solid #ddd; padding: 4px 8px; }',
    body: `<main><h1>Dashboard</h1><div class="tiles"><div class="tile"><b>128</b> builds</div><div class="tile"><b>97%</b> pass</div><div class="tile"><b>4</b> engines</div></div><div class="toolbar">${['Run', 'Stop', 'Export', 'Share'].map((t) => `<button>${t}</button>`).join('')}</div>${['Name', 'Region', 'Plan'].map((k) => `<div class="prow"><span>${k}</span><span>value</span></div>`).join('')}<table><tbody>${rows(6)}</tbody></table></main>`,
  },
);

export function renderPage(p) {
  return `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>${p.slug}</title>`
    + '<script>if (navigator.userAgent.includes("Firefox")) document.documentElement.classList.add("ff");</script>'
    + `<style>${BASE_CSS}${p.css || ''}</style></head><body>${p.header ?? HEADER()}${p.body}${FOOTER}</body></html>`;
}
