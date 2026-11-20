"""The PyPI wrapper in a fresh venv, as a user gets it: run() over the
planted pages (Task 84).

    <venv>/bin/python test/wrappers/run_planted.py <base-url> <out> <engines>
"""

import os
import sys
from pathlib import Path

base, out, engines = sys.argv[1:4]
if os.environ.get("OMNISNAP_BINARY"):
    sys.exit("OMNISNAP_BINARY is set: this checks the binary inside the wheel")
# The installed wheel, not the repository checkout next to this file.
sys.path = [p for p in sys.path if Path(p).resolve() != Path(__file__).resolve().parent]
import omnisnap  # noqa: E402

planted = Path(__file__).resolve().parents[2] / "testdata" / "planted"
pages = [f"{base}/{p.stem}" for p in sorted(planted.glob("[0-9][0-9]-*.html"))]
print("binary:", omnisnap.resolve_binary())
summary = omnisnap.run(pages=pages, engines=engines.split(","), out=out, allow_small_pages=True)
print(f"{len(summary['results'])} results in {out}")
