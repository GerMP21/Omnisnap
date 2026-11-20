"""Build the five PyPI platform wheels (SPEC-wrappers.md, Task 84).

Each wheel holds the pure-Python API and the one binary for its platform,
under the platform tag pip matches on. Run after `go run ./tools/release
--version <tag> --stage`, which puts the binaries in
packages/pypi/omnisnap/bin/<key>/:

    python tools/release/wheels.py --out dist-py

Building from packages/pypi directly would put every staged binary in one
universal (py3-none-any) wheel: 19 MB, over the 15 MB target, and tagged for
every platform. So each wheel is built from a copy holding only its binary,
then retagged.

Retagging uses `wheel tags`: "Make a new wheel with given tags from an
existing wheel." (https://wheel.readthedocs.io/en/stable/reference/wheel_tags.html).
A native executable is a valid reason for a platform tag: "Built distributions
may be platform specific for reasons other than C extensions, such as by
including a native executable invoked as a subprocess."
(https://packaging.python.org/en/latest/specifications/platform-compatibility-tags/)

The macOS tags say 13.0: "For Go 1.27 and later: macOS 13 Ventura or newer"
(https://go.dev/wiki/MinimumRequirements); an older tag would let pip install
a binary that cannot start. The Linux binaries are built with CGO_ENABLED=0.

Needs `build` and `wheel` (pip install build wheel). Publishes nothing.
"""

from __future__ import annotations

import argparse
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PYPI = ROOT / "packages" / "pypi"

# Platform key (the wrapper's bin/<key>/) -> wheel platform tag.
PLATFORMS = {
    "linux-x64": "manylinux_2_28_x86_64",
    "linux-arm64": "manylinux_2_28_aarch64",
    "darwin-x64": "macosx_13_0_x86_64",
    "darwin-arm64": "macosx_13_0_arm64",
    "win32-x64": "win_amd64",
}

MAX_BYTES = 15_000_000  # PRD distribution target, per platform package


def exe_name(key: str) -> str:
    return "omnisnap.exe" if key.startswith("win32") else "omnisnap"


def build_one(key: str, tag: str, out: Path) -> Path:
    binary = PYPI / "omnisnap" / "bin" / key / exe_name(key)
    if not binary.is_file():
        sys.exit(f"wheels: {binary} is missing: run `go run ./tools/release --version <tag> --stage` first")
    with tempfile.TemporaryDirectory() as tmp:
        src = Path(tmp) / "pypi"
        shutil.copytree(PYPI, src, ignore=shutil.ignore_patterns(
            "bin", "__pycache__", "*.egg-info", "build", "dist", "test"))
        dest = src / "omnisnap" / "bin" / key
        dest.mkdir(parents=True)
        shutil.copy2(binary, dest / exe_name(key))
        built = Path(tmp) / "built"
        res = subprocess.run([sys.executable, "-m", "build", "--wheel", "--outdir", str(built), str(src)],
                             capture_output=True, text=True)
        if res.returncode != 0:
            sys.exit(f"wheels: build {key} failed:\n{res.stdout}{res.stderr}")
        # The omnisnap.bin warning is expected (pyproject.toml); any other is not.
        warned = [l for l in (res.stdout + res.stderr).splitlines()
                  if "WARNING" in l and "'omnisnap.bin' is absent" not in l]
        if warned:
            print(f"wheels: build {key} warned:\n" + "\n".join(warned))
        (wheel,) = built.glob("*.whl")
        res = subprocess.run([sys.executable, "-m", "wheel", "tags", f"--platform-tag={tag}", "--remove", str(wheel)],
                             check=True, capture_output=True, text=True)
        tagged = built / res.stdout.strip().splitlines()[-1]
        out.mkdir(parents=True, exist_ok=True)
        final = out / tagged.name
        shutil.move(str(tagged), final)
    return final


def check(wheel: Path, key: str, tag: str) -> list[str]:
    """What a published wheel must hold: its tag in the name and in WHEEL,
    exactly one binary (this platform's, executable), under the size target."""
    problems = []
    if not wheel.name.endswith(f"-py3-none-{tag}.whl"):
        problems.append(f"file name is not tagged py3-none-{tag}")
    with zipfile.ZipFile(wheel) as z:
        names = z.namelist()
        bins = [n for n in names if "/bin/" in n and not n.endswith("/")]
        if bins != [f"omnisnap/bin/{key}/{exe_name(key)}"]:
            problems.append(f"binaries {bins}; want only {key}'s")
        else:
            mode = z.getinfo(bins[0]).external_attr >> 16
            if not key.startswith("win32") and not mode & 0o111:
                problems.append(f"{bins[0]} is not executable in the wheel (mode {mode:o})")
        meta = next(n for n in names if n.endswith(".dist-info/WHEEL"))
        tags = [l.split(":", 1)[1].strip() for l in z.read(meta).decode().splitlines() if l.startswith("Tag:")]
        if tags != [f"py3-none-{tag}"]:
            problems.append(f"WHEEL says Tag: {tags}")
    if wheel.stat().st_size >= MAX_BYTES:
        problems.append(f"{wheel.stat().st_size} bytes; the target is under {MAX_BYTES}")
    return problems


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--out", default=str(ROOT / "dist-py"), help="output directory (cleared first)")
    ap.add_argument("--only", choices=sorted(PLATFORMS), help="build one platform's wheel (CI builds the runner's own)")
    args = ap.parse_args()
    out = Path(args.out).resolve()
    if out.exists():
        for old in out.glob("omnisnap-*.whl"):  # a stale wheel must not be uploaded with these
            old.unlink()
    failed = False
    for key, tag in PLATFORMS.items():
        if args.only and key != args.only:
            continue
        wheel = build_one(key, tag, out)
        problems = check(wheel, key, tag)
        print(f"{wheel.name}  {wheel.stat().st_size} bytes  {'ok' if not problems else 'FAILED: ' + '; '.join(problems)}")
        failed = failed or bool(problems)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
