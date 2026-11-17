// Omnisnap page stabilizer. Evaluated in the page as ONE expression
// (CDP Runtime.evaluate / BiDi script.evaluate with awaitPromise).
// Kept engine-agnostic: the Go core will embed this file unchanged.
// Returns a JSON string so deep results never hit protocol depth limits.
(async () => {
  const t0 = performance.now();
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const frames = () => new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
  const within = (p, ms) => Promise.race([p, sleep(ms)]);

  // Same layout viewport in every engine: no root scrollbar and no reserved
  // gutter (Chrome keeps a 15px gutter for `scrollbar-gutter: stable` even
  // with --hide-scrollbars; Firefox's overlay scrollbars never reserve one).
  const norm = document.createElement('style');
  norm.setAttribute('data-omnisnap', 'normalize');
  norm.textContent = 'html { scrollbar-width: none !important; }';
  document.documentElement.append(norm);

  if (document.readyState !== 'complete') {
    await within(new Promise((r) => addEventListener('load', r, { once: true })), 15000);
  }
  if (document.fonts) await within(document.fonts.ready, 5000);

  // Stepped scroll: an instant jump to the bottom skips IntersectionObserver
  // callbacks for content in the middle, so lazy content would never load.
  const step = Math.max(200, Math.floor(innerHeight * 0.8));
  const maxH = Math.min(document.documentElement.scrollHeight, 30000);
  for (let y = 0; y < maxH; y += step) {
    scrollTo(0, y);
    await frames();
    await sleep(60);
  }
  scrollTo(0, 0);
  await frames();

  const pending = [...document.images].filter((i) => !i.complete);
  await within(
    Promise.all(pending.map((i) => new Promise((r) => {
      i.addEventListener('load', r, { once: true });
      i.addEventListener('error', r, { once: true });
    }))),
    5000,
  );
  if (document.fonts) await within(document.fonts.ready, 3000);

  // Finite animations jump to their end state; infinite ones freeze at t=0.
  let finished = 0;
  let frozen = 0;
  if (document.getAnimations) {
    for (const a of document.getAnimations()) {
      try {
        const end = a.effect && a.effect.getComputedTiming().endTime;
        if (Number.isFinite(end)) { a.finish(); finished++; } else { a.pause(); a.currentTime = 0; frozen++; }
      } catch (_) { /* some animations refuse finish(); ignore */ }
    }
  }
  await frames();

  // Freeze JS timers for the rest of the capture, so geometry and screenshot
  // describe the SAME moment (a slideshow advanced between the two on
  // synaptech-solutions.com). Works over every protocol, WebDriver Classic
  // included; it does not make engines show the same slide (that needs clock
  // control before page scripts run). The original rAF stays available for
  // Omnisnap's own scrolling.
  window.__omnisnapRAF = window.requestAnimationFrame.bind(window);
  const lastId = setTimeout(() => {}, 0);
  for (let i = 0; i <= lastId; i++) { clearTimeout(i); clearInterval(i); }
  const noop = () => 0;
  window.setTimeout = noop;
  window.setInterval = noop;
  window.requestAnimationFrame = noop;

  return JSON.stringify({ ms: Math.round(performance.now() - t0), finished, frozen, timersCleared: lastId, scrolledTo: maxH, pendingImages: pending.length });
})()
