"""Assert the planted gates on a summary a wrapper returned (Tasks 83, 84).

    python test/wrappers/check_summary.py <summary.json> [testdata/planted/expectations.json]

The gates are SPEC-ground-truth.md's success criteria 1 and 2, the ones
TestPlantedGates holds on the golden captures, applied to what run() returned
in a fresh install:

1. "Planted recall in the Report tier >= 90%"; "every planted page within its
   Report budget" (maxFindings).
2. "0 Report causes on control pages"; KB fixtures 0 Report. Native
   form-control pages are not scored (genuine engine differences).

And every page is captured ("ok"). A planted page without a Report cause is
named as a miss but does not fail on its own: these are live captures on the
runner's fonts and engines, not the Linux goldens the per-page gate runs on.
Stdlib only.
"""

import json
import sys
from pathlib import Path

RECALL = 0.9


def main() -> int:
    summary = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    exp_path = Path(sys.argv[2]) if len(sys.argv) > 2 else Path(__file__).resolve().parents[2] / "testdata" / "planted" / "expectations.json"
    pages = {p["slug"]: p for p in json.loads(exp_path.read_text(encoding="utf-8"))["pages"]}
    problems, misses = [], []
    seen = set()
    planted = hit = 0
    for r in summary["results"]:
        slug = r["page"]["slug"]
        match = [s for s in pages if slug == s or slug.endswith("-" + s)]
        if not match:
            problems.append(f"{slug}: not a planted page")
            continue
        page = pages[match[0]]
        seen.add(page["slug"])
        kind, report, causes = page["kind"], r["stats"]["report"], len(r.get("causes") or [])
        if r["status"] != "ok":
            problems.append(f"{slug} ({r['engine']['engine']}): status {r['status']}: {r.get('warnings')}")
            continue
        if kind == "planted":
            planted += 1
            if report > 0:
                hit += 1
            else:
                misses.append(slug)
            budget = page.get("maxFindings")
            if budget is not None and causes > budget:
                problems.append(f"{slug}: {causes} causes; budget {budget}")
        elif kind in ("control", "kb") and report > 0:
            problems.append(f"{slug}: {report} Report cause(s) on a {kind} page")
    if missing := sorted(set(pages) - seen):
        problems.append(f"no result for {missing}")
    recall = hit / planted if planted else 0.0
    if recall < RECALL:
        problems.append(f"planted recall {hit}/{planted} ({recall:.0%}); want >= {RECALL:.0%}")
    print(f"{len(summary['results'])} results; planted recall {hit}/{planted} ({recall:.0%}); problems: {len(problems)}")
    for m in misses:
        print(f"  miss (no Report cause): {m}")
    for p in problems:
        print("  " + p)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
