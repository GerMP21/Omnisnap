"""Assert the planted gates on a summary a wrapper returned (Tasks 83, 84).

    python test/wrappers/check_summary.py <summary.json> [testdata/planted/expectations.json]

Every page is captured ("ok"), a planted page has a Report-tier cause, and a
control, knowledge-base or native page has none: the rule TestPlantedGates
holds, checked here on what run() returned in a fresh install. Stdlib only.
"""

import json
import sys
from pathlib import Path


def main() -> int:
    summary = json.loads(Path(sys.argv[1]).read_text(encoding="utf-8"))
    exp_path = Path(sys.argv[2]) if len(sys.argv) > 2 else Path(__file__).resolve().parents[2] / "testdata" / "planted" / "expectations.json"
    kinds = {p["slug"]: p["kind"] for p in json.loads(exp_path.read_text(encoding="utf-8"))["pages"]}
    problems = []
    seen = set()
    for r in summary["results"]:
        slug = r["page"]["slug"]
        match = [s for s in kinds if slug == s or slug.endswith("-" + s)]
        if not match:
            problems.append(f"{slug}: not a planted page")
            continue
        seen.add(match[0])
        kind, report = kinds[match[0]], r["stats"]["report"]
        if r["status"] != "ok":
            problems.append(f"{slug} ({r['engine']['engine']}): status {r['status']}: {r.get('warnings')}")
        elif (kind == "planted") != (report > 0):
            problems.append(f"{slug}: kind {kind} with {report} Report cause(s)")
    if missing := sorted(set(kinds) - seen):
        problems.append(f"no result for {missing}")
    planted = sum(1 for k in kinds.values() if k == "planted")
    print(f"{len(summary['results'])} results; planted pages with a Report: "
          f"{sum(1 for r in summary['results'] if r['stats']['report'] > 0)}/{planted}; problems: {len(problems)}")
    for p in problems:
        print("  " + p)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
