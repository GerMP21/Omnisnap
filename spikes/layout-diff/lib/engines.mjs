// Engine config shared by capture.mjs, warm.mjs and fontcheck.mjs.
// Three protocols, as in the product: CDP (Blink), BiDi (Gecko), and
// W3C WebDriver Classic (WebKit, the Safari family).
import fs from 'node:fs';
import fsp from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import puppeteer from 'puppeteer-core';
import { launchWebDriver } from './webdriver.mjs';

const HERE = path.dirname(fileURLToPath(import.meta.url));
export const PROFILES = path.join(HERE, '..', 'profiles');
const env = process.env;

// On Linux, Blink resolves generic families from its own defaults (Arial ->
// Liberation Sans, Times New Roman -> Liberation Serif) while Gecko and
// WebKitGTK ask fontconfig, so the SAME machine renders `sans-serif` in
// different fonts. FONT_PARITY (default on) points the fontconfig engines at
// Blink's fonts. FONT_PARITY=0 measures the machine as configured.
export const FONT_PARITY = env.FONT_PARITY !== '0';
const PARITY_FONTS = {
  serif: env.PARITY_SERIF || 'Liberation Serif',
  'sans-serif': env.PARITY_SANS || 'Liberation Sans',
  monospace: env.PARITY_MONO || 'Noto Sans Mono',
};
const firefoxFontPrefs = () => {
  const prefs = {};
  for (const lang of ['x-western', 'x-unicode']) {
    for (const [generic, font] of Object.entries(PARITY_FONTS)) prefs[`font.name.${generic}.${lang}`] = font;
  }
  return prefs;
};
const webkitFontArgs = () => [
  `--default-font-family=${PARITY_FONTS.serif}`,
  `--serif-font-family=${PARITY_FONTS.serif}`,
  `--sans-serif-font-family=${PARITY_FONTS['sans-serif']}`,
  `--monospace-font-family=${PARITY_FONTS.monospace}`,
];

// No disk cache: a crawl would otherwise write hundreds of MB per browser
// session (and WSL's virtual disk never gives that space back to Windows).
// The memory cache stays; caching never changes layout.
const BLINK_ARGS = ['--hide-scrollbars', '--no-first-run', '--no-default-browser-check', '--force-device-scale-factor=1', '--disk-cache-size=1', '--media-cache-size=1'];

export const ENGINES = {
  chrome: { family: 'blink', protocol: 'cdp', browser: 'chrome', executablePath: env.CHROME_PATH || '/usr/bin/google-chrome', args: BLINK_ARGS },
  chromium: { family: 'blink', protocol: 'cdp', browser: 'chrome', executablePath: env.CHROMIUM_PATH || '/usr/bin/chromium-browser', args: BLINK_ARGS },
  // Branded browsers run AS SHIPPED, from a warmed template profile when one
  // exists (node warm.mjs): a fresh profile has not downloaded its blocking
  // lists yet, so it would behave like plain Chromium.
  brave: { family: 'blink', protocol: 'cdp', browser: 'chrome', executablePath: env.BRAVE_PATH || '/usr/bin/brave-browser', args: BLINK_ARGS, template: true },
  edge: { family: 'blink', protocol: 'cdp', browser: 'chrome', executablePath: env.EDGE_PATH || '/usr/bin/microsoft-edge-stable', args: BLINK_ARGS, template: true },
  firefox: {
    family: 'gecko',
    protocol: 'bidi',
    browser: 'firefox',
    executablePath: env.FIREFOX_PATH || '/usr/bin/firefox',
    // Overlay scrollbars so the layout viewport matches Blink's.
    extraPrefsFirefox: {
      'ui.useOverlayScrollbars': 1,
      'widget.gtk.overlay-scrollbars.enabled': true,
      'layout.css.devPixelsPerPx': '1.0',
      // No disk cache (see BLINK_ARGS).
      'browser.cache.disk.enable': false,
      'browser.cache.disk.capacity': 0,
      ...(FONT_PARITY ? firefoxFontPrefs() : {}),
    },
  },
  webkit: {
    family: 'webkit',
    protocol: 'webdriver',
    executablePath: env.WEBKIT_DRIVER || '/usr/bin/WebKitWebDriver',
    binary: env.WEBKIT_BROWSER || '/usr/libexec/webkitgtk-6.0/MiniBrowser',
    args: ['--automation', ...(FONT_PARITY ? webkitFontArgs() : [])],
  },
};

const exists = (p) => { try { fs.accessSync(p, fs.constants.X_OK); return true; } catch (_) { return false; } };
export function availableEngines(filter) {
  return Object.entries(ENGINES)
    .filter(([name, cfg]) => (!filter || filter.includes(name)) && exists(cfg.executablePath) && (!cfg.binary || exists(cfg.binary)))
    .map(([name]) => name);
}

// Copy the warmed template so every run starts from the same state and the
// template itself never accumulates cookies, cache or engagement scores.
export async function profileFromTemplate(name) {
  const template = path.join(PROFILES, name);
  try { await fsp.access(template); } catch (_) { return null; }
  const dir = await fsp.mkdtemp(path.join(os.tmpdir(), `omnisnap-${name}-`));
  await fsp.cp(template, dir, { recursive: true, filter: (src) => !/Singleton(Lock|Socket|Cookie)$/.test(src) });
  return dir;
}

export async function launch(name, { cold = false } = {}) {
  const cfg = ENGINES[name];
  if (cfg.protocol === 'webdriver') return launchWebDriver(cfg);
  const opts = { browser: cfg.browser, executablePath: cfg.executablePath, args: cfg.args, extraPrefsFirefox: cfg.extraPrefsFirefox, headless: true };
  let profile = null;
  if (cfg.template && !cold) profile = await profileFromTemplate(name);
  if (profile) opts.userDataDir = profile;
  let browser;
  try {
    browser = await puppeteer.launch(opts);
  } catch (e) {
    if (cfg.family !== 'blink' || !/sandbox/i.test(String(e))) throw e;
    console.warn(`  ${name}: sandbox unavailable, retrying with --no-sandbox`);
    browser = await puppeteer.launch({ ...opts, args: [...cfg.args, '--no-sandbox'] });
  }
  browser.omnisnap = { name, protocol: cfg.protocol, profile: profile ? 'warm template' : (cfg.template ? 'cold (no template: run warm.mjs)' : 'fresh') };
  const close = browser.close.bind(browser);
  browser.close = async () => { await close(); if (profile) await fsp.rm(profile, { recursive: true, force: true }).catch(() => {}); };
  return browser;
}
