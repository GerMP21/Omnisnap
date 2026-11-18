# Product Requirements Document (PRD)

**Project Name:** Omnisnap

**Document Version:** 1.6.0 (1.0.0 text unchanged; amendments in the [Changelog](#changelog))

**Target Runtimes:** Node.js (≥\ge≥ 18.0.0) and Python (≥\ge≥ 3.10)

**Distribution Target:** npm and PyPI (<15 MB package footprint)

**Supported Platforms:** Windows 10/11, macOS 12+ (Apple Silicon & Intel), Linux (Ubuntu, Debian, Fedora, Arch)

## Changelog

### 1.6.0 (2026-11-18): Edge attribution withheld; install verification fixed

- **Edge tracking prevention:** measured 0 blocked requests headless and 0 headed (Edge 157 on a private Weston display) on the tracker page Brave blocks. Edge stays a supported Blink browser for layout, but `blockedBy` attribution is not claimed for it until the Windows CI artifact (`branded-blocked`) reports blocked requests (SPEC-branded.md).
- **`omnisnap install`:** the SHA-512 lookup used the local file name while Mozilla's SHA512SUMS lists the archive path (`linux-x86_64/en-US/...`), so a real install failed after the signature check; the macOS and Windows downloads are also renamed locally. Fixed by verifying against the release-directory path.
- **Pinned release key:** updated to Mozilla's current key (new signing subkey `827E6586…678E767AA3`; the 2025 subkey is revoked).


> The 1.0.0 text below is kept as written. Where it conflicts with an entry here, **this changelog wins**.
> Direction and v1 scope: [One-pager](ideas/omnisnap.md).

### 1.5.0 (2026-11-08): v1 specified and planned

- **Specs:** v1 is split into 17 modules, each with its own spec, in [specs/](../specs/CAPABILITY-MAP.md), with shared conventions in [specs/CONVENTIONS.md](../specs/CONVENTIONS.md). Where a spec and the 1.0.0 text differ, the spec wins.
- **Plan:** 86 tasks in 12 phases in [tasks/](../tasks/plan.md). Milestones are unchanged: M0 capture + viewer (tasks 1–38), M1 layout diff (39–76), M2 CI report (77–86).
- **Repository:** the project lives in the author's own GitHub organization; the Go module path follows it.
- **Round 6 of the spike** added mutation testing (realistic bugs injected into real pages and labeled automatically) and a LightGBM tree model. Its training run is paused for disk space; task 76 decides the model's role.
- **Docs consolidated:** the origin conversation, the two early architecture research documents and the precision research prompt were removed from `docs/`. They are superseded by the one-pager, this changelog and the specs; the originals remain in the Obsidian vault.

### 1.4.0 (2026-11-07): Automated ground truth, design intent, larger regression suite

Evidence: [spike README](../spikes/layout-diff/README.md) (round 5) and [Scientific grounding](SCIENTIFIC_GROUNDING.md).

**Withdrawn**

- **Manual labels (rounds 1–4) were unreliable and are withdrawn**, with every figure derived from them: the precision figures in 1.2.0 *Open* and 1.3.0 *Open*, the label programme (1.2.0 #8) and the condition "until labeled data shows it" (1.3.0 #4). The label files and the labeling UI were deleted from the spike. Human labels are not part of the ground truth.

**New requirements**

| # | Requirement | Amends |
|---|---|---|
| 1 | **Automated ground truth.** (a) **Mutation testing on real pages:** realistic cross-engine bug operators (missing feature fallbacks, different defaults, sizing rules) injected into one engine only, with the target element known. This gives recall per operator and positive examples. (b) **Null pairs:** the same engine twice, and two Blink browsers. This gives the false-alarm rate and negative examples. (c) **Natural cross-engine differences** stay unlabeled and are learned from as positive-unlabeled data. Precision is reported exactly on mutations vs null pairs, and estimated on natural differences; never from manual labels | 1.2.0 #8 |
| 2 | **A small local learned model, not an LLM:** a compact vision network (a siamese CNN or small vision transformer) on paired element crops plus geometry and evidence features, trained on the automated data. Inference must run locally in the Go core (ONNX), within the package size budget (target < 10 MB quantized). Gradient-boosted trees on the numeric evidence are the baseline it must beat on held-out sites | 1.2.0 #8, 1.3.0 #4 |
| 3 | **Automated crawl for training data:** about 500 sites drawn from a public top-sites list, filtered automatically (reachable, no bot challenge, renders in every engine, page states agree), captured in every engine with probes, plus mutated variants | new |
| 4 | **Design intent as a detector:** relations that at least two engine families keep (shared edges, equal sizes of repeated components, even spacing, grid columns, no overlap) are the design's intent. One engine breaking one by ≥ 2 px (≥ 8% for sizes) is reported, the baseline included. Pages whose states diverge are skipped | §7, 1.3.0 #3 |
| 5 | **Planted regression suite grows with realistic bug patterns** (card CTA alignment, grid columns, toolbar alignment, list rhythm, equal-height tiles, overlap, ellipsis, line-clamp, aspect-ratio, nav wrapping, code scrolling, visibility, table layout, flex min-width) **and intent controls.** The planted pages stay: they are the regression suite and the threshold calibration set | 1.2.0 #3 |

**Open**

- Round 5 results on known answers: planted recall 24/24 (Wilson 86–100%), 0 Report causes on controls, design intent 6/6 planted with 0 false alarms; intent fires on 11 of 71 real-site engine pairs (5 violations by Chrome). The judge stays a ranking signal: as the tier it cost recall (19/24).
- Mutation realism decides what the model learns: the operators must come from real cross-engine bug classes, not arbitrary CSS changes.
- Precision on natural cross-engine differences can only be estimated (positive-unlabeled), not counted.

### 1.3.0 (2026-11-07): Scientific grounding, round 4

Evidence: [Scientific grounding](SCIENTIFIC_GROUNDING.md) and the [spike README, Round 4](../spikes/layout-diff/README.md#round-4-2026-11-07-eight-concepts-in-tandem).

**New requirements**

| # | Requirement | Amends |
|---|---|---|
| 1 | **Causal probes:** each capture re-measures the page with line-height pinned to whole pixels, one font forced and form controls neutralized. A cause that vanishes under a probe is attributed to that mechanism, and the probe verifies the fix | §6, §7, 1.2.0 #4 |
| 2 | **Overflow by mode:** clipped (`hidden`/`clip`) and scrollbar (`auto`/`scroll`) overflow are hard Report consequences. Visible overflow is a soft consequence. Content pushed beyond a viewport is deliberate hiding | 1.2.0 #3 |
| 3 | **Evidence per cause in the report:** mechanism and verified fix, perceptual events, which engine is odd (needs a third engine family), page self-calibration, ink agreement vs the page's noise floor, and how many other sites show the same cause | §9 |
| 4 | **Judge stays experimental:** a weak-supervision label model may rank causes, but the tier stays rule-based until labeled data shows it is at least as precise. Next labels go to causes where the two disagree | 1.2.0 #8 |

**Open**

- Labeled precision: rules 2/3 [21–94%], judge 1/2 [10–91%]; 22 usable labels can't separate them.
- Design-intent checks (near-miss alignments) never fired on 23 crawled sites.

### 1.2.0 (2026-11-07): Layout-diff spike and precision research

Evidence: [spike README](../spikes/layout-diff/README.md) (rounds 1–3, labeled data in `spikes/layout-diff/labeled-findings.json`) and the [precision research report](research/Omnisnap%20Precision%20Research%20Report%20-%20From%20Every%20Genuine%20Difference%20to%20Differences%20a%20Developer%20Wants.md).

**Validated by the spike**

| Area | Result | Amends |
|---|---|---|
| Element matching + root-cause attribution | 10/10 planted bugs found as one finding at the right element; 0 findings on 61 bug-free control pages across 5 engines; 97.3–100% of elements matched on real sites | §7 (confirms 1.1.0 "Comparison engine") |
| Protocol drivers | CDP (Chrome, Chromium, Brave, Edge), BiDi (Firefox) and **WebDriver Classic (WebKitGTK)**, using a minimal HTTP client and scroll-and-stitch; 168/168 captures succeeded | §2 |
| Branded Blink browsers | Pixel-identical to Chrome (SSIM 1.0000) on every test page and the owner's site; they only differ where product features act (Brave blocks trackers only from a **warmed** profile: 0 fresh vs 3 warmed) | §3, 1.1.0 "Branded browsers" |
| Whole-page pixel diff (SSIM/DSSIM) | **Rejected as a detector:** bug-free control pages fail DSSIM 0.02 (0.027 / 0.045) because Skia and Firefox rasterize the same glyphs differently; two planted bugs sit below that noise | §7.2–7.3 |

**New requirements**

| # | Requirement | Amends |
|---|---|---|
| 1 | **Normalize the environment before comparing:** generic font-family parity across engines (Blink uses Liberation, fontconfig engines Noto on the same Linux machine), root scrollbar/gutter hidden, viewport fitted exactly (WebDriver Classic needs a measure-and-correct loop), CJK and other script fonts detected (missing glyphs render differently per engine) | §2, §5, NFR-5 |
| 2 | **Capture consistency:** capture all engines at nearly the same moment, freeze JS timers after stabilization, re-measure geometry after the screenshot and warn if the page changed; capture 2+ runs per engine and mask what changes between them | §5, §8 |
| 3 | **Report causes, not elements:** findings sharing a mechanism are one cause with ranked instances; causes split into **Report** (a visible consequence: presence, visibility, overflow/protrusion, truncation, line-count change, interactive target moved, structural change) and **Info** (expected engine differences, no visible consequence). Precision is measured per Report-tier cause with Wilson confidence intervals | §7, §9 |
| 4 | **Knowledge base of expected engine differences**, each with a detection signature, explanation and fix (v0: fractional line-height rounding, `line-height: normal`, text advance, long-token line breaking, form-control defaults); entries carry engine versions and are re-validated on fixture pages | §7, new |
| 5 | **Pixels only where geometry is blind:** SSIM on aligned crops of graphics elements (img/svg/canvas/video), ±3 px offset search, dynamic elements skipped; page-level DSSIM only as a "different page states" alarm | §7 |
| 6 | **WebKit on Linux** (WebKitGTK) runs headless through a private Weston display and is a WebKit *layout* proxy, not Safari *pixels* (it renders with Skia; Safari uses CoreGraphics). Real Safari still comes from macOS | §2.3, §8 |
| 7 | **Clock control** (fake timers before page scripts) is possible over CDP and BiDi, **not over WebDriver Classic**; Safari needs an opt-in record/replay proxy that injects the clock. Deferred until page-state divergence is common in the corpus | §5 |
| 8 | **Label programme before any learned ranker:** ≥ 100 cause-level labels from ≥ 2 raters (rubric, crops of the same page state), CSS-mutation recall tests; a small logistic-regression ranker in pure Go only after ≥ 150 labels | new |

**Open**

- **Precision is not yet at target:** per finding, 46.7% of labeled findings were real. At the cause level (round 3), the Report tier has 2/3 labeled causes real (Wilson 21–94%), no false positives, and 4 real items demoted to Info. 28 non-Stripe labels give a 95% interval of roughly 13–43% around 25%, so 80% cannot be demonstrated without more labels.
- Edge's tracking prevention downloads its lists as a component, and blocked nothing under headless automation even after a forced update.
- Real Safari 27 may lay out fractional line-heights with subpixel precision (WebKitGTK 2.50 does not); verify on a macOS runner before shipping that knowledge-base entry's text.

### 1.1.0 (2026-11-06): Idea refinement

**Product decisions**

| Area | Change | Amends |
|---|---|---|
| Direction & v1 | v1 is the local **Inspector** plus a **report-only GitHub Action**. v1 ships at M2. | §2, §5 |
| Users & success | Primary users: frontend developers (local) and CI teams. Agency/QA becomes secondary. Success metric: OSS adoption. | §2 |
| Architecture | **One Go binary**, shipped through platform-specific npm and PyPI packages. The Node and Python SDKs become thin wrappers: they write `job.json`, run the binary, and read NDJSON progress plus `summary.json`. The §10.2/§10.3 API shapes stay as the wrapper surface. Process cleanup uses `golang.org/x/sys/windows` (Job Objects) and process groups, replacing ctypes and Node native bindings. | Header (Target Runtimes), §4.1, §10 |
| Comparison engine | **Element-level layout diff is the primary comparison.** The injected JS extracts geometry; elements are matched by stable identifiers, then by aligning each element's children in order; differences are reported where they start (geometry relative to the parent); a second baseline-engine run masks dynamic content. SSIM is used only on matched element crops. No whole-page pixel diff across engines. | §1.2, §7 |
| Branded browsers | Run **as shipped** by default. `--neutral` is opt-in and replaces `--preserve-shields`. Each browser gets a warmed template profile, copied fresh for every run. Filter-list versions are recorded in `summary.json`. The user's real profile is never used (Chrome 136+ also ignores remote-debugging switches on the default data dir). Each difference is explained by a per-browser list of blocked requests (`ERR_BLOCKED_BY_CLIENT`). | §3, §10.1 |
| Browser downloads | Zero-download stays the default. New opt-in `omnisnap install firefox` downloads the official Mozilla build into a cache. | §1.2, NFR-1 |
| CI | New GitHub Action: Ubuntu, Windows and macOS runners, one merged PR comment, **report-only**. Real Safari comes from the macOS runners. The pass/fail gate is deferred until report mode has produced noise data. | §2, new feature |
| Report schema | `host` metadata moves to each result (it was per run), so bundles from several machines can be merged. New `job.json` input and `omnisnap merge` command. The schema URL moves off `omnisnap.dev`, which a third party has registered, to **`omnisnap.org`** (registered 2026-11-17; the `omnisnap-schemas` Worker serves the `$id`s). | §9.1 |
| Milestones | Replaced by: **Spike** (throwaway puppeteer-core validation of layout diff) → **M0** capture + viewer → **M1** layout diff → **M2** CI report. | §5 |
| Scope cuts | Arc (in maintenance mode), Tor Browser (disables remote automation by design), mobile browsers, an embedded AI/vision model, GitLab/Bitbucket/self-hosted runners. The pass/fail gate is deferred. | §1.1, §2, §3 |
| License | **AGPL-3.0-or-later** (changed from Apache-2.0 on 2026-11-18, before any release) | new |

**Corrections to the 1.0.0 text**

- **§1.2, §7.3:** the formula labeled ΔE₀₀ is actually CIE76 (ΔE\*ab). CIEDE2000 is considerably more complex.
- **Feature 1.1 (Windows discovery):**
  - Opera GX registers `opera.exe` in `StartMenuInternet`, not `launcher.exe` (observed on the author's machine).
  - Chromium registers under a key with a hashed suffix (`Chromium.<HASH>`), so match on the prefix.
  - The legacy `IEXPLORE.EXE` entry must be filtered out.
  - `chrome.exe --version` prints nothing on Windows. Read the version from file metadata or the versioned install folder.
- **Feature 1.3 (Linux discovery):** Ubuntu's default Firefox is a Snap with a private `/tmp`, so profiles must live somewhere the snap can read. WebKitGTK needs the WebDriver from the `webkit2gtk` package plus a display (Xvfb in CI).
- **Features 1.2, 2.3, NFR-5 (Safari):** Safari needs a one-time `safaridriver --enable`, which asks for an admin password. It has no headless mode (it opens a visible window), so NFR-5 cannot apply to Safari. Zero-config does not hold for WebKit.
- **Feature 2.2 (Gecko):** `remote.active-protocols = 3` is legacy. Firefox disabled its CDP implementation (v129+), and WebDriver BiDi is on by default.
- **Feature 3 (Brave):** Shields are built in and on by default. A clean profile doesn't remove them; that is exactly what makes Brave differ from Chrome. `--disable-brave-extension` very likely doesn't stop blocking. In a fresh profile the filter lists haven't downloaded yet, hence warmed template profiles. *(Still to be confirmed by the spike.)*
- **Feature 5 (stabilization):**
  - Jumping straight to the bottom of the page and back doesn't trigger `IntersectionObserver` for content in the middle, so scroll in steps.
  - The fetch/XHR patch must be injected before the page's own scripts run (CDP `Page.addScriptToEvaluateOnNewDocument`, BiDi `script.addPreloadScript`). WebDriver Classic can't do that, so Safari has no network-quiet signal.

## 1. Executive Summary & Product Vision

### 1.1 Vision Statement

Omnisnap is an open-source, zero-configuration cross-browser snapshot and visual verification orchestrator. It executes directly against all browsers installed on the host operating system—including Chrome, Firefox, Safari, Edge, Brave, Opera, Vivaldi, Arc, and open-source derivatives—without requiring multi-gigabyte browser downloads, driver binaries (`chromedriver`, `geckodriver`), or heavy end-to-end testing frameworks.

### 1.2 Core Differentiation

Traditional frameworks (Playwright, Puppeteer, Selenium) require hundreds of megabytes of platform-bundled binaries, rely on synthetic user interaction dispatchers, and suffer from high false-positive rates during cross-browser diffing caused by operating system font antialiasing. Omnisnap replaces this model with:

- **Zero-Download Footprint:** Uses native local browsers already present on the machine.
    
- **Direct Protocol Execution:** Implements native Chrome DevTools Protocol (CDP) over WebSockets for Blink, native W3C WebDriver BiDi over WebSockets for Gecko, and native HTTP REST for WebKit.
    
- **Perceptual Differentiation:** Replaces pixel-matching with Structural Similarity Index Measure (SSIM), Disimilitud Estructural (DSSIM), and CIELAB ΔE00\Delta E_{00}ΔE00​ color distance filters that eliminate font-smoothing noise.
    
- **Injected DOM Auditing:** Detects structural UI bugs (horizontal overflow, element overlap, clipped containers) directly within the layout tree prior to screenshot capture.
    

## 2. Target Personas & Use Cases

- **Frontend & Design System Engineers:** Execute `npx omnisnap http://localhost:3000` prior to git commit to view instant side-by-side renders across 6–10 installed browsers in under 10 seconds.
    
- **CI/CD Automation Engineers:** Run an automated visual gate on Linux runners (`epiphany`, `chromium`, `firefox`) that outputs standardized JSON metrics and fails builds on genuine layout regressions.
    
- **Agency & Cross-Browser QA Teams:** Audit customer-facing web applications across niche commercial browsers (Brave Shields, Opera GX, Vivaldi) to verify that default browser features do not break layout integrity.
    

## 3. Comprehensive Feature Specifications

### Feature 1: Universal Zero-Config Binary Discovery Engine

The discovery engine must detect all installed browsers on the host within 300 ms without manual configuration.

#### 1.1 Windows Discovery Subsystem

- **Registry Client Scanning:** Asynchronously queries `HKEY_LOCAL_MACHINE\SOFTWARE\Clients\StartMenuInternet` and `HKEY_CURRENT_USER\SOFTWARE\Clients\StartMenuInternet`.
    
- **WOW64 Translation:** Queries `HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Clients\StartMenuInternet` for 32-bit browsers running on 64-bit Windows.
    
- **Path Resolution:** For each subkey found (e.g., `Google Chrome`, `Firefox`, `OperaStable`, `Brave`), resolves `shell\open\command` to extract the executable path.
    
- **Secondary Heuristic:** Queries `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\<name>.exe` and inspects `%LocalAppData%\Programs`, `%ProgramFiles%`, and `%ProgramFiles(x86)%` for portable or user-scope installations.
    

#### 1.2 macOS Discovery Subsystem

- **LaunchServices Spotlight Queries:** Executes `/usr/bin/mdfind` targeting bundle identifiers:
    
    - Google Chrome: `kMDItemCFBundleIdentifier == "com.google.Chrome"`
        
    - Microsoft Edge: `kMDItemCFBundleIdentifier == "com.microsoft.edgemac"`
        
    - Brave: `kMDItemCFBundleIdentifier == "com.brave.Browser"`
        
    - Vivaldi: `kMDItemCFBundleIdentifier == "com.vivaldi.Vivaldi"`
        
    - Arc: `kMDItemCFBundleIdentifier == "company.thebrowser.Browser"`
        
    - Opera / Opera GX: `kMDItemCFBundleIdentifier == "com.operasoftware.Opera*"`
        
    - Mozilla Firefox / Nightly: `kMDItemCFBundleIdentifier == "org.mozilla.firefox*"`
        
    - Floorp: `kMDItemCFBundleIdentifier == "jp.ablaze.floorp"`
        
    - Waterfox: `kMDItemCFBundleIdentifier == "net.waterfox.waterfox"`
        
- **Direct Path Fallback:** Scans `/Applications` and `~/Applications` for application bundles.
    
- **Safari Driver Verification:** Verifies presence and executable permissions of `/usr/bin/safaridriver`.
    

#### 1.3 Linux Discovery Subsystem

- **XDG Desktop Application Scanner:** Parses `.desktop` files located in `/usr/share/applications/` and `~/.local/share/applications/` filtering for `Categories=*WebBrowser*` and extracting executable arguments from `Exec=`.
    
- **Package Manager Container Detection:**
    
    - Executes `flatpak list --app --columns=application` and matches against known browser IDs (`org.mozilla.firefox`, `com.google.Chrome`, `com.brave.Browser`, `com.valvesoftware.SteamLink`, etc.).
        
    - Executes `snap list` and extracts entries matching `firefox`, `chromium`, `opera`.
        
- **PATH Resolution:** Iterates `$PATH` looking for binaries: `google-chrome`, `google-chrome-stable`, `chromium`, `chromium-browser`, `microsoft-edge-stable`, `brave-browser`, `firefox`, `epiphany`, `floorp`, `waterfox`.
    
- **WebKitGTK Verification:** Validates availability of `WebKitWebDriver` for Linux-native WebKit automation.
    

### Feature 2: Protocol-Level Driver Layer (No Intermediate Proxies)

#### 2.1 Blink Driver (Chrome, Chromium, Edge, Brave, Opera, Vivaldi, Arc)

- **Transport:** Single persistent JSON-RPC 2.0 connection over WebSocket to the browser target.
    
- **Initialization Flow:**
    
    1. Spawns binary with `--remote-debugging-port=0`, `--headless=new`, `--hide-scrollbars`, `--force-device-scale-factor=1`, `--no-first-run`, `--no-default-browser-check`, and a dedicated temporary `--user-data-dir`.
        
    2. Polls `[http://127.0.0.1](http://127.0.0.1):<port>/json/version` over HTTP to obtain `webSocketDebuggerUrl`.
        
    3. Sends `Page.enable` and `DOM.enable`.
        
    4. Sends `Emulation.setDeviceMetricsOverride` setting explicit viewport width, height, and scale factor 1.01.01.0.
        
- **Capture Execution:**
    
    1. Issues `Page.navigate` and waits for quiescence.
        
    2. Issues `Page.getLayoutMetrics` to retrieve `cssContentSize` (`width`, `height`).
        
    3. Issues `Page.captureScreenshot` with `captureBeyondViewport: true` and `clip` set to `cssContentSize` dimensions to capture a single-pass full-page image without scrolling.
        
    4. Automatically chunks captures into vertical blocks of 8,000 pixels if document height exceeds 16,384 pixels, then stitches the buffers in memory.
        

#### 2.2 Gecko Driver (Firefox, Floorp, Waterfox, LibreWolf)

- **Transport:** JSON-RPC 2.0 over WebSocket via Firefox's native Remote Agent (`/session`) implementing W3C WebDriver BiDi. No `geckodriver` binary permitted.
    
- **Initialization Flow:**
    
    1. Creates temporary directory containing a generated `user.js` file configuring `remote.active-protocols = 3` and disabling telemetry and first-run prompts.
        
    2. Acquires free ephemeral port and launches binary with `--headless`, `--profile <dir>`, `--remote-debugging-port <port>`, and `--no-remote`.
        
    3. Establishes WebSocket connection to `ws://127.0.0.1:<port>/session`.
        
    4. Sends `session.new` with `{ "capabilities": {} }`.
        
    5. Queries context tree via `browsingContext.getTree` to obtain root browsing context ID.
        
    6. Sends `browsingContext.setViewport` configuring explicit dimensions and `devicePixelRatio: 1.0`.
        
- **Capture Execution:**
    
    1. Issues `browsingContext.navigate` with `"wait": "complete"`.
        
    2. Evaluates stabilization script via `script.callFunction`.
        
    3. Issues `browsingContext.captureScreenshot` with `"origin": "document"` to produce native full-page capture.
        

#### 2.3 WebKit Driver (macOS Safari & Linux WebKitGTK)

- **Transport:** W3C WebDriver Classic REST API over HTTP/1.1.
    
- **macOS Flow:** Spawns `/usr/bin/safaridriver -p <port>`, verifies health, and sends `POST /session`.
    
- **Linux Flow:** Spawns `WebKitWebDriver -p <port>` and sends `POST /session` with browser set to `epiphany`.
    
- **Capture Execution (Scroll-and-Stitch Pipeline):**
    
    1. Standard W3C command `GET /session/{id}/screenshot` only captures the visible viewport.
        
    2. Injects script via `POST /session/{id}/execute/sync` querying `document.documentElement.scrollHeight`, `window.innerHeight`, and finding all elements matching `position: fixed` or `position: sticky`.
        
    3. Captures frame 0.
        
    4. Hides fixed and sticky elements via `element.style.visibility = 'hidden'` to prevent duplicated headers.
        
    5. Scrolls document incrementally using `window.scrollTo(0, offsetY)` and captures successive viewport frames after waiting two animation frames.
        
    6. Restores original CSS visibility styles.
        
    7. Stitches vertical image buffers and crops boundary overlaps using native image libraries (Sharp / Pillow).
        

### Feature 3: Browser Quirk Mitigation & Sandbox Isolation

To prevent consumer features and extensions from causing artificial layout discrepancies, the orchestrator applies runtime overrides:

- **Brave Shields Mitigation:** By default, Brave blocks third-party trackers, fonts, and scripts. The launcher passes `--disable-brave-extension` or provisions a clean profile to prevent discrepancies with Chrome. A `--preserve-shields` CLI flag is provided to allow developers to intentionally test how Brave Shields affects layout.
    
- **Opera & Opera GX Multi-Process Handling:** On Windows, Opera's `launcher.exe` spawns separate child processes and exits immediately. The launcher ignores parent process exit and polls the loopback HTTP port `/json/version` directly until the CDP endpoint responds or times out (10s).
    
- **Vivaldi & Arc UI Shell Suppression:** Forces `--headless=new` and `--hide-scrollbars` to ensure browser shell elements (Vivaldi side panels, Arc spaces/docks) are not rendered into the document canvas.
    
- **Anti-Fingerprinting / Letterboxing Mitigation:** In LibreWolf and Tor Browser, default settings add canvas noise and letterbox viewports to round dimensions. The launcher overrides these preferences via injected `user.js` (`privacy.resistFingerprinting = false`) during visual test sessions.
    

### Feature 4: Process Lifecycle & Guaranteed Zombie Cleanup

The system guarantees that zero browser processes, child renderers, GPU processes, or temporary profile directories linger after test completion or sudden termination (e.g., `SIGINT`, `Ctrl+C`).

#### 4.1 Windows Job Objects Subsystem

- Calls Win32 API functions `CreateJobObject`, `SetInformationJobObject`, and `AssignProcessToJobObject` via `ctypes` in Python or native bindings in Node.js.
    
- Applies `JOBOBJECT_EXTENDED_LIMIT_INFORMATION` with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`.
    
- Result: If the parent orchestrator process terminates unexpectedly, Windows terminates all child and grandchild browser processes at the kernel level.
    

#### 4.2 Unix (macOS & Linux) Process Group Subsystem

- Launches all child browser processes detached into their own process groups (`detached: true` in Node.js `child_process.spawn`; `preexec_fn=os.setsid` in Python `subprocess.Popen`).
    
- Shutdown sequence:
    
    1. Sends `SIGTERM` to the negative process group ID (`-pid`).
        
    2. Waits 500 ms for graceful release of locks.
        
    3. Sends `SIGKILL` to `-pid` if any sub-processes remain in `/proc`.
        
- Global Signal Traps: Registers handlers for `SIGINT`, `SIGTERM`, `SIGHUP`, and uncaught runtime exceptions to trigger tree termination before exiting.
    

#### 4.3 Profile Storage Cleanup

- All profiles reside in temporary directories (`omnisnap_<uuid>`).
    
- A shutdown hook deletes the temporary profile directories from disk.
    

### Feature 5: Deterministic Page Stabilization & Quiescence Pipeline

To eliminate transient blank frames, layout shifts, and missing web fonts, the driver executes a mandatory synchronization pipeline inside the browser context before taking any capture:

1. **Document Readiness Check:** Evaluates `document.readyState === 'complete'`.
    
2. **Network Quiescence Monitor:** Monitors in-flight network activity. The page must maintain zero pending network requests for a minimum continuous window of 500 ms.
    
3. **Web Font Loading Synchronization:** Awaits the resolution of `document.fonts.ready` to ensure web fonts are fully rasterized, preventing FOUT and FOIT.
    
4. **Animation Settlement:** Queries `document.getAnimations()`. For all running CSS transitions, CSS animations, and Web Animations API instances with finite duration, forces them to their final frame via `animation.finish()`.
    
5. **Lazy-Load Trigger (Scroll Sweep):** Performs an instantaneous vertical scroll sweep to `document.body.scrollHeight` and back to `(0, 0)`. This activates `IntersectionObserver` listeners and triggers downloads of lazy-loaded images.
    
6. **Compositor Flush:** Waits for two consecutive animation frame cycles via `requestAnimationFrame(() => requestAnimationFrame(resolve))` to confirm layout reflows and paints have completed in the rendering pipeline.
    

### Feature 6: Injected DOM Layout Anomaly Auditor

Prior to screenshot capture, the orchestrator executes an in-page DOM audit to detect structural layout bugs. This audit runs in memory with zero package weight:

|Bug Category|Detection Algorithm|Mathematical Condition|Reported Payload|
|---|---|---|---|
|**Horizontal Overflow**|Compares root scroll width to viewport client width.|`scrollWidth > clientWidth`|Offending DOM nodes exceeding viewport boundaries, CSS selectors, and computed overflow widths.|
|**Clipping Faults**|Checks elements inside containers with `overflow: hidden` or `overflow: clip`.|Child AABB⊈AABB \not\subseteqAABB⊆ Parent AABBAABBAABB|Parent/child selector pair, clipped pixel dimensions, and truncated text snippets.|
|**Element Collisions**|Evaluates non-nested visible elements in the same stacking context.|Standard Axis-Aligned Bounding Box (AABB) intersection formula: (Aleft<Bright)∧(Aright>Bleft)∧(Atop<Bbottom)∧(Abottom>Btop)(A_{\text{left}} < B_{\text{right}}) \land (A_{\text{right}} > B_{\text{left}}) \land (A_{\text{top}} < B_{\text{bottom}}) \land (A_{\text{bottom}} > B_{\text{top}})(Aleft​<Bright​)∧(Aright​>Bleft​)∧(Atop​<Bbottom​)∧(Abottom​>Btop​)|Conflicting DOM nodes, overlapping pixel area, and computed z-indices.|
|**Broken Media**|Scans all `<img>` and `<svg>` nodes in the layout.|`img.naturalWidth === 0` or missing image source|Image source URL, selector, and display bounding box.|

### Feature 7: Cross-Browser Perceptual Diffing Engine

#### 7.1 Geometry Normalization

- Standardizes all captures to a base width (e.g., 1920 px) and sets `devicePixelRatio: 1.0`.
    
- Crops compared images to matching canvas dimensions based on the intersection of their bounding boxes.
    

#### 7.2 Structural Similarity Evaluation (SSIM / DSSIM)

- Divides images into 8×88 \times 88×8 pixel windows.
    
- Evaluates luminance (lll), contrast (ccc), and structural correlation (sss) using the standard SSIM equation:
    
    SSIM(x,y)=(2μxμy+C1)(2σxy+C2)(μx2+μy2+C1)(σx2+σy2+C2)\text{SSIM}(x, y) = \frac{(2\mu_x\mu_y + C_1)(2\sigma_{xy} + C_2)}{(\mu_x^2 + \mu_y^2 + C_1)(\sigma_x^2 + \sigma_y^2 + C_2)}SSIM(x,y)=(μx2​+μy2​+C1​)(σx2​+σy2​+C2​)(2μx​μy​+C1​)(2σxy​+C2​)​
    
- Calculates Structural Dissimilarity:
    
    DSSIM(x,y)=1−SSIM(x,y)2\text{DSSIM}(x, y) = \frac{1 - \text{SSIM}(x, y)}{2}DSSIM(x,y)=21−SSIM(x,y)​
    
- Compares target browser images against Google Chrome as the baseline.
    

#### 7.3 Antialiasing Filtering Subsystem

- Converts RGB pixel values in divergent regions to the perceptually uniform CIELAB (L∗,a∗,b∗L^*, a^*, b^*L∗,a∗,b∗) color space.
    
- Calculates the color difference:
    
    ΔE00=(ΔL∗)2+(Δa∗)2+(Δb∗)2\Delta E_{00} = \sqrt{(\Delta L^*)^2 + (\Delta a^*)^2 + (\Delta b^*)^2}ΔE00​=(ΔL∗)2+(Δa∗)2+(Δb∗)2![](data:image/svg+xml;utf8,<svg%20xmlns="http://www.w3.org/2000/svg"%20width="400em"%20height="1.28em"%20viewBox="0%200%20400000%201296"%20preserveAspectRatio="xMinYMin%20slice"><path%20d="M263,681c0.7,0,18,39.7,52,119
    c34,79.3,68.167,158.7,102.5,238c34.3,79.3,51.8,119.3,52.5,120
    c340,-704.7,510.7,-1060.3,512,-1067
    l0%20-0
    c4.7,-7.3,11,-11,19,-11
    H40000v40H1012.3
    s-271.3,567,-271.3,567c-38.7,80.7,-84,175,-136,283c-52,108,-89.167,185.3,-111.5,232
    c-22.3,46.7,-33.8,70.3,-34.5,71c-4.7,4.7,-12.3,7,-23,7s-12,-1,-12,-1
    s-109,-253,-109,-253c-72.7,-168,-109.3,-252,-110,-252c-10.7,8,-22,16.7,-34,26
    c-22,17.3,-33.3,26,-34,26s-26,-26,-26,-26s76,-59,76,-59s76,-60,76,-60z
    M1001%2080h400000v40h-400000z"></path></svg>)​
    
- If a pixel differs from baseline but sits within 1.5 pixels of a high-contrast boundary edge and has ΔE00<2.3\Delta E_{00} < 2.3ΔE00​<2.3 (the visual _Just Noticeable Difference_ threshold), it is classified as subpixel font smoothing or vector antialiasing and excluded from the failure count.
    
- Remaining divergent pixels represent genuine rendering bugs and are highlighted in magenta (`#FF00FF`) on a visual heatmap overlay.
    

### Feature 8: Asymmetric Concurrency & Task Scheduling

Due to platform-specific driver constraints, the execution scheduler partitions capture jobs into two streams:

- **Parallel Stream (Blink & Gecko):** Processes Chrome, Edge, Brave, Opera, Vivaldi, Arc, and Firefox concurrently across ephemeral ports up to the host machine's hardware thread limit.
    
- **Strict Serial Stream (WebKit on macOS):** `/usr/bin/safaridriver` does not permit concurrent active automation sessions on macOS. The scheduler routes Safari and Safari Technology Preview captures through a dedicated, single-threaded FIFO queue to prevent session conflicts.
    
- **Graceful Degradation:** If Safari is requested on Linux or Windows, the engine skips WebKit, logs an explanatory notice in the report, and marks the status as `skipped: platform_unsupported` without aborting the overall test suite.
    

### Feature 9: Reporting Subsystem

#### 9.1 Machine-Readable `summary.json`

Every execution outputs a structured JSON summary containing execution metadata, timing, DOM anomalies, and comparison scores:

JSON

```
{
  "$schema": "https://omnisnap.dev/v1/schema.json",
  "runId": "run_01j7b9c48e8",
  "targetUrl": "https://example.com",
  "timestamp": "2026-11-06T22:58:00Z",
  "host": {
    "os": "darwin",
    "arch": "arm64",
    "release": "25.1.0"
  },
  "config": {
    "viewport": { "width": 1920, "height": 1080, "dpr": 1.0 },
    "fullPage": true,
    "diffThreshold": 0.02
  },
  "results": [
    {
      "browser": "Google Chrome",
      "engine": "Blink",
      "version": "134.0.6998.35",
      "status": "success",
      "durationMs": 1420,
      "screenshotPath": "captures/chrome_1920.png",
      "domAudit": {
        "hasHorizontalOverflow": false,
        "brokenMedia": [],
        "clippingFaults": [],
        "elementCollisions": []
      }
    },
    {
      "browser": "Mozilla Firefox",
      "engine": "Gecko",
      "version": "136.0",
      "status": "success",
      "durationMs": 1810,
      "screenshotPath": "captures/firefox_1920.png",
      "domAudit": {
        "hasHorizontalOverflow": true,
        "brokenMedia": [],
        "clippingFaults": [
          {
            "selector": "div.pricing-card > p.desc",
            "reason": "Text clipped by overflow:hidden container"
          }
        ],
        "elementCollisions": []
      }
    }
  ],
  "comparisons": [
    {
      "baseline": "Google Chrome",
      "target": "Mozilla Firefox",
      "ssim": 0.9882,
      "dssim": 0.0059,
      "divergentPixelCount": 420,
      "diffHeatmapPath": "diffs/chrome_vs_firefox.png",
      "passed": true
    }
  ]
}
```

#### 9.2 Interactive Standalone `report.html`

The HTML report is fully self-contained (no external CDN dependencies) and embeds:

- **Matrix Inspection View:** Renders all browser captures in a synchronized, pan-and-zoom responsive grid.
    
- **Interactive Split Slider:** Lets users drag a vertical divider between the baseline image (Chrome) and any target browser image to inspect micro-alignments.
    
- **Diff Heatmap Toggle:** Overlays divergent pixels highlighted in magenta on top of the original image.
    
- **DOM Anomaly Drawer:** Lists all detected layout faults (overflows, collisions, clipping) with direct links that pan the viewer to the exact pixel coordinates of the offending DOM node.
    

### Feature 10: CLI & Programmatic SDK Specification

#### 10.1 CLI Commands & Options

- **Discovery Command:**
    
    Bash
    
    ```
    omnisnap detect
    ```
    
    Lists all detected browsers, their rendering engines, resolved executable paths, and versions without running tests.
    
- **Execution Command:**
    
    Bash
    
    ```
    omnisnap <url> [options]
    ```
    
    - `--browsers <list>`: Comma-separated browser filter (e.g., `chrome,brave,firefox,safari`). Defaults to `all`.
        
    - `--engines <list>`: Limits execution to engine representatives (e.g., `blink,gecko,webkit`).
        
    - `--viewports <list>`: Comma-separated resolutions (e.g., `1920x1080,390x844`).
        
    - `--full-page`: Enables full-page capture. Defaults to `true`.
        
    - `--diff-threshold <float>`: Maximum allowed DSSIM before failing the test run (default: `0.02`).
        
    - `--output <dir>`: Target report directory (default: `./omnisnap-report`).
        
    - `--preserve-shields`: Prevents automatic disabling of Brave Shields.
        
    - `--json`: Outputs only `summary.json` to stdout for scripting.
        

#### 10.2 Node.js Programmatic API

TypeScript

```
import { Omnisnap } from 'omnisnap';

const orchestrator = new Omnisnap({
  outputDir: './reports',
  diffThreshold: 0.02
});

const installedBrowsers = await orchestrator.detectBrowsers();

const runResults = await orchestrator.capture({
  url: 'https://staging.app.internal',
  browsers: ['chrome', 'firefox', 'safari'],
  viewports: [{ width: 1920, height: 1080 }],
  fullPage: true,
  runDomAudit: true
});

if (!runResults.passed) {
  process.exit(1);
}
```

#### 10.3 Python Programmatic API

Python

```
from omnisnap import Omnisnap, Viewport

orchestrator = Omnisnap(
    output_dir="./reports",
    diff_threshold=0.02
)

detected = orchestrator.detect_browsers()

results = orchestrator.capture(
    url="https://staging.app.internal",
    browsers=["chrome", "firefox", "safari"],
    viewports=[Viewport(width=1920, height=1080)],
    full_page=True,
    run_dom_audit=True
)

if not results.passed:
    raise SystemExit(1)
```

## 4. Non-Functional Requirements (NFRs)

- **NFR-1: Package Size Limit:** The core npm package and PyPI wheel must each remain under 15 MB compressed. Browser binaries, WebAssembly heavyweights, and container images must not be bundled.
    
- **NFR-2: Discovery Speed:** System-wide browser discovery must complete in under 300 ms across Windows, macOS, and Linux.
    
- **NFR-3: Zero Residual State:** Unplanned process aborts (`SIGINT`, uncaught exceptions) must terminate all spawned browser processes and clean up temporary profiles within 1 second.
    
- **NFR-4: Memory Safety:** Image diffing operations must stream large buffers to disk or downsample dynamically when total image dimensions exceed 16K×16K16\text{K} \times 16\text{K}16K×16K pixels to prevent memory exhaustion.
    
- **NFR-5: Headless Consistency:** All Blink instances must run with `--headless=new` and all Gecko instances with `--headless` to match headed rendering output exactly.
    

## 5. Technical Implementation Milestones

- **Milestone 1 (Driver & Protocol Primitives):** Implement raw WebSocket/HTTP protocol clients for CDP, BiDi, and WebDriver Classic. Verify viewport and full-page screenshot capture on Chrome, Firefox, and Safari on macOS.
    
- **Milestone 2 (Discovery & Process Lifecycle):** Implement Windows Registry scanning, macOS Spotlight queries, and Linux desktop file/package parsers. Add Win32 Job Objects and POSIX process group cleanup handlers.
    
- **Milestone 3 (Quiescence & In-Page Audit):** Implement font synchronization, animation settlement, network idle tracking, and the in-memory DOM anomaly detector.
    
- **Milestone 4 (Perceptual Diffing & Report):** Implement SSIM/DSSIM calculation with CIELAB ΔE00\Delta E_{00}ΔE00​ antialiasing filtering. Generate the standalone `report.html` and `summary.json`.
    
- **Milestone 5 (Cross-Platform Verification & Packaging):** Test and validate execution against Windows 11, macOS 15, and Ubuntu 24.04 environments. Verify that final package sizes on npm and PyPI remain below 15 MB.