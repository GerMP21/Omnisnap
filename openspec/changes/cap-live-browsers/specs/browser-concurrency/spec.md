# Spec Delta

## Purpose

Bounds the memory a capture job takes from the machine it runs on: how many browsers a job keeps alive at once, the order engines are captured in when a job has more engines than that cap, and how Omnisnap's browsers rank against the user's own processes when memory runs out.

## ADDED Requirements

### Requirement: At most N live browsers per job
A capture job SHALL never have more than N of its browsers running at the same time, where N is the live-browser cap. A browser counts as live from its launch until its process tree has been killed. A relaunch after a crash replaces a browser and does not add one.

#### Scenario: Eight engines with the default cap
- **GIVEN** a machine where discovery finds eight engines and the job does not restrict `engines`
- **WHEN** the user runs `omnisnap <url>` without `--max-browsers`
- **THEN** at no point during the job are more than three of Omnisnap's browsers running
- **AND** every one of the eight engines is captured, or listed as skipped with its reason

#### Scenario: A crashed browser is relaunched within the cap
- **GIVEN** a job with three live browsers and the cap at 3
- **WHEN** one browser crashes and the crash policy relaunches it
- **THEN** the number of live browsers never exceeds three

### Requirement: The cap defaults to 3 and is set with --max-browsers
The live-browser cap SHALL default to 3. `omnisnap <url>` and `omnisnap run <job>` SHALL accept `--max-browsers N`, where N is a positive integer. The cap SHALL NOT be a field of the job file.

#### Scenario: The user raises the cap
- **WHEN** the user runs `omnisnap <url> --max-browsers 8` on a machine with eight engines
- **THEN** all eight browsers are live together and captured as one group

#### Scenario: An invalid cap is a usage error
- **WHEN** the user passes `--max-browsers 0`, a negative number or a non-integer
- **THEN** Omnisnap exits with code 1 (usage) before launching any browser, and the message names the flag and the accepted values

#### Scenario: A job file cannot set the cap
- **WHEN** a job file contains a `maxBrowsers` field
- **THEN** the job is rejected as invalid against the job schema, exactly as any unknown field is

### Requirement: Jobs within the cap are unchanged
A job whose launched engines number N or fewer SHALL capture in the same order, write the same bundle and emit the same progress events, in the same order, as before the cap existed.

#### Scenario: Three engines with the default cap
- **GIVEN** a job with Chrome, Firefox and WebKitGTK and the cap at 3
- **WHEN** the job runs on the planted suite
- **THEN** every browser is launched before the first page and closed after the last, as before
- **AND** the progress event sequence matches the one recorded before the change, apart from timestamps and durations

### Requirement: Engines beyond the cap run in consecutive groups
When a job has more engines than the cap, Omnisnap SHALL capture in consecutive groups. A group launches engines in job order, baseline first, until N are live or none remain, then captures every page with them. All its browsers close before the next group launches.

#### Scenario: Five engines with the cap at 3
- **GIVEN** a job with engines chrome (baseline), firefox, webkitgtk, edge, brave in that order, all launchable, and the cap at 3
- **WHEN** the job runs
- **THEN** chrome, firefox and webkitgtk capture every page first, and their browsers are closed
- **AND** then edge and brave are launched and capture every page

#### Scenario: The baseline is always in the first group
- **GIVEN** a job whose `baseline` is listed last in `engines`, with more engines than the cap
- **WHEN** the job runs
- **THEN** the baseline is launched in the first group

### Requirement: Launch failures do not use up the cap
An engine whose browser fails to launch SHALL be recorded as skipped with its reason, and SHALL NOT count toward the cap. The group launches the next engine in job order instead.

#### Scenario: Two engines fail to launch on a CI runner
- **GIVEN** engines chrome, chromium, edge, firefox, webkitgtk, where chromium and edge fail to launch, and the cap at 3
- **WHEN** the job runs
- **THEN** chrome, firefox and webkitgtk are live together in one group
- **AND** chromium and edge are listed as skipped with "launch failed" and their errors

### Requirement: Runs stay interleaved within a group
Within a group, for every page, Omnisnap SHALL capture run a of every live engine before run b of any of them, with the protocol lanes in parallel as before.

#### Scenario: Two runs in the second group
- **GIVEN** a job with `runs: 2`, five engines and the cap at 3
- **WHEN** the second group captures a page
- **THEN** run a of each of its engines is captured before run b of either of them

### Requirement: Reachability is decided once, by the baseline
A page that the baseline's run a finds unreachable in the first group SHALL NOT be captured by any later group. Later groups SHALL capture only pages the first group found reachable.

#### Scenario: A bot wall seen in the first group
- **GIVEN** a job with five engines, the cap at 3, and a page whose baseline run a hits a bot wall
- **WHEN** the second group runs
- **THEN** it opens no session for that page
- **AND** the bundle records the page as unreachable once, with the baseline's reason

### Requirement: Each page reports done once
For each page, Omnisnap SHALL emit exactly one page-start progress event, before its first capture, and exactly one page-done event, after its last capture in the last group that captures it.

#### Scenario: Progress for a grouped job
- **GIVEN** a job with two pages, five engines and the cap at 3, run with `--json`
- **WHEN** the job completes
- **THEN** the progress stream holds exactly one page-start event and one page-done event for each page
- **AND** every capture event of a page comes between its page-start and page-done events

### Requirement: Omnisnap's browsers are killed first under memory pressure on Linux
On Linux, every process Omnisnap launches, and every descendant of those processes, SHALL run with the highest out-of-memory score adjustment (`oom_score_adj` 1000). Omnisnap's own process SHALL keep its score.

#### Scenario: Browser processes carry the score
- **GIVEN** a job running on Linux with Chrome, Firefox and WebKitGTK
- **WHEN** the first page is being captured
- **THEN** every process in each browser's process tree (renderers and content processes included), and each driver and the private Weston, reads 1000 in `/proc/<pid>/oom_score_adj`
- **AND** the `omnisnap` process reads the value it started with

#### Scenario: The kernel kills an Omnisnap browser, not the user's
- **GIVEN** a Linux machine under memory pressure, with a user browser and an Omnisnap job both running
- **WHEN** the kernel's out-of-memory killer runs
- **THEN** it kills a process of one of Omnisnap's browsers
- **AND** the crash policy retries the capture in a fresh browser, or reports the engine as lost, and the job still writes its bundle

### Requirement: Processes Omnisnap did not start are never touched
Omnisnap SHALL change the out-of-memory score only of processes it launched and their descendants. On Windows and macOS it SHALL change no process priority or memory setting.

#### Scenario: The user's own browser keeps its score
- **GIVEN** the user's Firefox running on Linux before an Omnisnap job starts
- **WHEN** the job runs and finishes
- **THEN** the `oom_score_adj` of every process of the user's Firefox is the value it had before the job

#### Scenario: Windows and macOS
- **WHEN** a job runs on Windows or macOS
- **THEN** no process has its priority or memory score changed by Omnisnap
