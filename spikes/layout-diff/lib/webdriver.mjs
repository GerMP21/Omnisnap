// Minimal W3C WebDriver Classic client for WebKit (WebKitGTK here, Safari's
// safaridriver in the product): plain HTTP, no driver library. Classic has
// no network events, no preload scripts and viewport-only screenshots, so
// full-page images are scroll-and-stitch (see captureWebDriver in capture.mjs).
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import path from 'node:path';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const which = (bin) => {
  for (const dir of (process.env.PATH || '').split(':')) {
    const p = path.join(dir, bin);
    try { fs.accessSync(p, fs.constants.X_OK); return p; } catch (_) { /* keep looking */ }
  }
  return null;
};
const killGroup = async (proc) => {
  try { process.kill(-proc.pid, 'SIGTERM'); } catch (_) { /* already gone */ }
  await sleep(400);
  try { process.kill(-proc.pid, 'SIGKILL'); } catch (_) { /* already gone */ }
};

// WebKitGTK has no headless mode of its own: give it a private, invisible
// Wayland display (Weston's headless backend) instead of the user's desktop.
// Bonus: we choose the screen size, so the window can always reach 1280x800.
// WEBKIT_HEADLESS=0 shows the windows instead.
async function startHeadlessDisplay() {
  if (process.env.WEBKIT_HEADLESS === '0') return null;
  const weston = which('weston');
  if (!weston) return null;
  const runtime = process.env.XDG_RUNTIME_DIR || `/run/user/${process.getuid()}`;
  const socket = `omnisnap-${process.pid}-${Date.now()}`;
  const proc = spawn(weston, ['--backend=headless', `--socket=${socket}`, '--width=1920', '--height=1200', '--idle-time=0'], {
    stdio: ['ignore', 'ignore', 'pipe'], detached: true, env: { ...process.env, XDG_RUNTIME_DIR: runtime },
  });
  let stderr = '';
  proc.stderr.on('data', (d) => { stderr += d; });
  const sock = path.join(runtime, socket);
  for (let i = 0; i < 60 && !fs.existsSync(sock); i++) await sleep(100);
  if (!fs.existsSync(sock)) { await killGroup(proc); throw new Error(`weston headless did not start: ${stderr.slice(0, 300)}`); }
  return { socket, runtime, stop: () => killGroup(proc) };
}
const freePort = () => new Promise((resolve, reject) => {
  const s = net.createServer();
  s.on('error', reject);
  s.listen(0, '127.0.0.1', () => { const { port } = s.address(); s.close(() => resolve(port)); });
});

export async function launchWebDriver(cfg) {
  const port = await freePort();
  let display = null;
  try { display = await startHeadlessDisplay(); } catch (e) { console.warn(`  webkit: ${e.message}; windows will be visible`); }
  if (!display && process.env.WEBKIT_HEADLESS !== '0') console.warn('  webkit: weston not installed, MiniBrowser windows will be visible (sudo dnf install -y weston)');
  const env = { ...process.env };
  if (display) {
    Object.assign(env, { WAYLAND_DISPLAY: display.socket, XDG_RUNTIME_DIR: display.runtime, GDK_BACKEND: 'wayland' });
    delete env.DISPLAY; // never fall back to the user's X/WSLg desktop
  }
  // Own process group, so the driver AND every browser it spawned die together.
  const proc = spawn(cfg.executablePath, [`--port=${port}`], { stdio: ['ignore', 'ignore', 'pipe'], detached: true, env });
  let stderr = '';
  proc.stderr.on('data', (d) => { stderr += d; });
  const base = `http://127.0.0.1:${port}`;

  const call = async (method, route, body) => {
    const res = await fetch(base + route, {
      method,
      headers: { 'content-type': 'application/json; charset=utf-8' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const json = await res.json().catch(() => ({}));
    if (!res.ok) throw new Error(`${method} ${route} -> ${res.status} ${JSON.stringify(json.value ?? json).slice(0, 300)}`);
    return json.value;
  };

  for (let i = 0; ; i++) {
    try { await call('GET', '/status'); break; } catch (e) {
      if (i > 60) throw new Error(`WebKitWebDriver did not start: ${stderr.slice(0, 400) || e.message}`);
      await sleep(100);
    }
  }

  const capabilities = {
    capabilities: {
      alwaysMatch: {
        browserName: 'MiniBrowser',
        'webkitgtk:browserOptions': { binary: cfg.binary, args: cfg.args },
      },
    },
  };

  let version = 'WebKitGTK';
  const driver = {
    omnisnap: { name: 'webkit', protocol: 'webdriver', profile: display ? 'fresh, headless (weston)' : 'fresh, VISIBLE windows' },
    version: async () => version,
    async newSession() {
      const v = await call('POST', '/session', capabilities);
      const id = v.sessionId;
      version = `${v.capabilities.browserName} ${v.capabilities.browserVersion || ''}`.trim();
      const s = (method, route, body) => call(method, `/session/${id}${route}`, body);
      await s('POST', '/timeouts', { script: 60000, pageLoad: 30000, implicit: 0 });
      return {
        navigate: (url) => s('POST', '/url', { url }),
        // execute/async works on every Classic implementation, whether or not
        // it awaits promises returned from execute/sync.
        exec: (expr) => s('POST', '/execute/async', {
          script: `const done = arguments[arguments.length - 1];\nPromise.resolve((\n${expr}\n)).then(done, (e) => done(JSON.stringify({ omnisnapError: String(e) })));`,
          args: [],
        }),
        screenshot: async () => Buffer.from(await s('GET', '/screenshot'), 'base64'),
        setWindow: (width, height) => s('POST', '/window/rect', { width, height }),
        getWindow: () => s('GET', '/window/rect'),
        close: () => s('DELETE', '').catch(() => {}),
      };
    },
    async close() {
      await killGroup(proc);
      if (display) await display.stop();
    },
  };
  return driver;
}
