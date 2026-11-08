// Build the automated crawl list (crawl-sites.json) from the public Tranco
// top-sites ranking (https://tranco-list.eu), walking it from the top and
// keeping sites a layout comparison can use:
//   - not infrastructure (CDNs, APIs, ad/tracking, DNS, update servers);
//   - not adult content or gambling (domain, title and page text checked);
//   - one site per brand (google.com, not also google.de);
//   - the homepage answers 200 with a real HTML page (>= 5 KB, a title);
//   - no bot challenge, login wall or redirect to a sign-in page;
//   - not already in corpus.json.
// capture.mjs crawl reads the result next to corpus.json's crawl list.
//
// usage: node sites.mjs [N=500]   (cache: $OMNISNAP_OUT or out/, tranco zip)

import { execFileSync } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = process.env.OMNISNAP_OUT || path.join(HERE, 'out');
const N = Number(process.argv[2] || 500);
const SCAN = 6000; // ranks to consider at most
const CONCURRENCY = 16;
const UA = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0 Safari/537.36';

const INFRA = /(^|[.-])(cdn|api|apis|static|assets|img|images|edge|dns|ntp|ocsp|crl|pki|ads?|adserver|pixel|metrics|telemetry|tracking|analytics|tags?|push|update|updates|gateway|mx|smtp|mail[0-9]*|login|auth|sso|accounts?)([.-]|$)/;
const INFRA_HOSTS = /(googleapis|gstatic|googleusercontent|googlevideo|doubleclick|googlesyndication|googleadservices|google-analytics|googletagmanager|gvt[12]|1e100|akamai|akadns|akamaiedge|edgekey|edgesuite|cloudfront|amazonaws|awsdns|azureedge|azurefd|trafficmanager|msedge|msftncsi|windowsupdate|live\.net|office\.net|fastly|cloudflare-dns|apple-dns|icloud-content|aaplimg|mzstatic|cdninstagram|fbcdn|fbsbx|tiktokcdn|ttwstatic|byteoversea|ibytedtos|scorecardresearch|criteo|rubiconproject|pubmatic|adnxs|casalemedia|openx|taboola|outbrain|demdex|omtrdc|everesttech|bidswitch|digicert|letsencrypt|sectigo|root-servers|nic\.|gtld|ntp\.org|pool\.ntp|xboxlive|playstation\.net|nintendo\.net|spotifycdn|scdn|nflxso|nflxvideo|nflximg|steamstatic|steamcontent|whatsapp\.net|yandex\.net|vk-cdn|userapi|qq\.com\.cn|alicdn|tbcdn|kwai|appsflyer|adjust|branch\.io|onesignal|sentry|newrelic|nr-data|datadoghq|hotjar|mixpanel|segment|amplitude|optimizely|bugsnag)/;
const ADULT = /(porn|xxx|sex|adult|xvideo|xhamster|onlyfans|chaturbate|stripchat|livejasmin|bongacams|cam4|camsoda|nude|naked|hentai|escort|erotic|fetish|brazzers|redtube|youporn|spankbang|xnxx|tube8|beeg|eporner|rule34|nsfw|milf|boobs|hookup|dating18|bokep|jav[a-z]*hd|missav|sexo|porno|imgsrc|polybuzz|crushon|janitorai|エロ|アダルト|セックス|成人|色情|порно|секс|سكس)/i;
// Gambling sites geo-block erratically and are not layout research material.
const GAMBLING = /(bet|casino|slot|poker|lottery|lotto|bookmaker|1xbet|stake\.|sportingbet|bwin|parimatch)/i;
const CHALLENGE = /cf-chl|challenge-platform|cf_chl_opt|captcha|just a moment|attention required|access denied|request unsuccessful|px-captcha|datadome|perimeterx|are you a robot|verify you are human|enable javascript and cookies/i;
const SIGNIN = /\/(login|signin|sign-in|sign_in|auth|account\/login|consent|sso)(\/|\?|$)/i;

const corpus = JSON.parse(await fs.readFile(path.join(HERE, 'corpus.json'), 'utf8'));
const known = new Set(Object.values(corpus).flat().filter((t) => t && t.url).map((t) => { try { return new URL(t.url).hostname.replace(/^www\./, ''); } catch (_) { return ''; } }));

// ----------------------------------------------------------- Tranco list
await fs.mkdir(OUT, { recursive: true });
const zip = path.join(OUT, 'tranco-top-1m.csv.zip');
let fresh = false;
try { const st = await fs.stat(zip); fresh = Date.now() - st.mtimeMs < 7 * 864e5; } catch (_) { /* none */ }
if (!fresh) {
  const res = await fetch('https://tranco-list.eu/top-1m.csv.zip', { headers: { 'user-agent': UA } });
  if (!res.ok) throw new Error(`tranco download: HTTP ${res.status}`);
  await fs.writeFile(zip, Buffer.from(await res.arrayBuffer()));
}
const csv = execFileSync('unzip', ['-p', zip], { maxBuffer: 64 * 1024 * 1024 }).toString('utf8');
const ranked = csv.split('\n').slice(0, SCAN).map((l) => l.trim().split(',')).filter((x) => x.length === 2).map(([rank, domain]) => ({ rank: Number(rank), domain }));
console.log(`tranco: ${ranked.length} ranks loaded`);

// ------------------------------------------------------------- filtering
const brand = (d) => { const parts = d.split('.'); return parts.length > 2 && parts.at(-2).length <= 3 ? parts.at(-3) : parts.at(-2); };
const seenBrand = new Set();
const candidates = [];
const dropped = { infra: 0, adult: 0, brand: 0, known: 0 };
for (const r of ranked) {
  const d = r.domain.toLowerCase();
  if (INFRA.test(d) || INFRA_HOSTS.test(d)) { dropped.infra++; continue; }
  if (ADULT.test(d)) { dropped.adult++; continue; }
  if (GAMBLING.test(d)) { dropped.gambling = (dropped.gambling || 0) + 1; continue; }
  if (known.has(d)) { dropped.known++; continue; }
  const b = brand(d);
  if (seenBrand.has(b)) { dropped.brand++; continue; }
  seenBrand.add(b);
  candidates.push({ ...r, domain: d });
}
console.log(`candidates after name filters: ${candidates.length} (dropped ${JSON.stringify(dropped)})`);

async function probe(c) {
  const ctl = new AbortController();
  const timer = setTimeout(() => ctl.abort(), 12000);
  try {
    const res = await fetch(`https://${c.domain}/`, { redirect: 'follow', signal: ctl.signal, headers: { 'user-agent': UA, accept: 'text/html', 'accept-language': 'en-US,en;q=0.8' } });
    if (res.status !== 200) return { why: `http ${res.status}` };
    if (!/text\/html/i.test(res.headers.get('content-type') || '')) return { why: 'not html' };
    const html = (await res.text()).slice(0, 400000);
    if (html.length < 5000) return { why: 'too small' };
    const title = (/<title[^>]*>([^<]{1,200})/i.exec(html) || [])[1]?.trim();
    if (!title) return { why: 'no title' };
    if (CHALLENGE.test(html.slice(0, 60000))) return { why: 'bot challenge' };
    if (ADULT.test(title) || (html.match(new RegExp(ADULT.source, 'gi')) || []).length > 5) return { why: 'adult' };
    if (GAMBLING.test(title)) return { why: 'gambling' };
    const final = new URL(res.url);
    if (SIGNIN.test(final.pathname) || /^(login|accounts?|auth|signin)\./.test(final.hostname)) return { why: 'sign-in redirect' };
    if (brand(final.hostname.replace(/^www\./, '')) !== brand(c.domain) && !final.hostname.endsWith(c.domain)) return { why: 'redirects to another site' };
    return { ok: true, url: final.href, title: title.slice(0, 80) };
  } catch (e) {
    return { why: e.name === 'AbortError' ? 'timeout' : 'network' };
  } finally {
    clearTimeout(timer);
  }
}

const accepted = [];
const reasons = {};
let next = 0;
async function worker() {
  while (next < candidates.length && accepted.length < N) {
    const c = candidates[next++];
    const r = await probe(c);
    if (r.ok && accepted.length < N) accepted.push({ slug: `t-${c.domain.replace(/[^a-z0-9]+/g, '-')}`, url: r.url, rank: c.rank, title: r.title });
    else if (!r.ok) reasons[r.why] = (reasons[r.why] || 0) + 1;
    if ((accepted.length + Object.values(reasons).reduce((s, x) => s + x, 0)) % 100 === 0) process.stdout.write(`  probed ${next}, accepted ${accepted.length}\r`);
  }
}
await Promise.all(Array.from({ length: CONCURRENCY }, worker));
accepted.sort((a, b) => a.rank - b.rank);
await fs.writeFile(path.join(HERE, 'crawl-sites.json'), JSON.stringify({
  generated: new Date().toISOString(),
  source: 'Tranco top-1M list (https://tranco-list.eu), downloaded for this crawl',
  criteria: 'not infrastructure or adult content; one site per brand; homepage 200 HTML >= 5 KB with a title; no bot challenge or sign-in redirect; not already in corpus.json',
  probed: next,
  rejected: reasons,
  sites: accepted,
}, null, 1));
console.log(`\naccepted ${accepted.length} of ${next} probed (rank ${accepted[0]?.rank}..${accepted.at(-1)?.rank}); rejected: ${JSON.stringify(reasons)}`);
console.log('wrote crawl-sites.json');
