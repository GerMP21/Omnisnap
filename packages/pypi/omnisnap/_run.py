"""Thin wrapper around the Omnisnap binary (SPEC-wrappers.md): write job.json,
run the binary, stream its NDJSON progress as dicts, return summary.json.
No capture or diff logic lives here; no runtime dependencies."""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path
from typing import Any, Callable, Iterable


class OmnisnapError(RuntimeError):
    """A failed binary run: the exit code and stderr travel with the error."""

    def __init__(self, message: str, *, code: int | None = None, stderr: str = "", args: Iterable[str] = ()):  # noqa: D401
        super().__init__(message)
        self.code = code
        self.stderr = stderr
        self.args_list = list(args)


def _platform_key() -> str:
    if sys.platform.startswith("linux"):
        os_name = "linux"
    elif sys.platform == "darwin":
        os_name = "darwin"
    elif sys.platform.startswith("win"):
        os_name = "win32"
    else:
        os_name = sys.platform
    machine = (os.uname().machine if hasattr(os, "uname") else os.environ.get("PROCESSOR_ARCHITECTURE", "")).lower()
    arch = "arm64" if machine in ("arm64", "aarch64") else "x64"
    return f"{os_name}-{arch}"


def resolve_binary() -> str:
    """The binary inside the wheel for this platform, then OMNISNAP_BINARY,
    then PATH."""
    override = os.environ.get("OMNISNAP_BINARY")
    if override:
        return override
    bundled = Path(__file__).resolve().parent / "bin" / _platform_key() / ("omnisnap.exe" if sys.platform.startswith("win") else "omnisnap")
    if bundled.is_file():
        return str(bundled)
    found = shutil.which("omnisnap.exe" if sys.platform.startswith("win") else "omnisnap")
    return found or ("omnisnap.exe" if sys.platform.startswith("win") else "omnisnap")


def run_binary(args: list[str], on_progress: Callable[[dict[str, Any]], None] | None = None) -> dict[str, Any]:
    """Run the binary; calls on_progress for each typed NDJSON event; returns
    stdout, stderr and the final done event (if any)."""
    binary = resolve_binary()
    try:
        proc = subprocess.Popen(
            [binary, *args],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            encoding="utf-8",
            errors="replace",
            bufsize=1,
        )
    except OSError as exc:  # binary missing or not executable
        raise OmnisnapError(f"cannot run {binary}: {exc}", args=args) from exc
    assert proc.stdout is not None
    done: dict[str, Any] | None = None
    for line in proc.stdout:
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue  # not NDJSON: ignore
        if isinstance(event, dict) and event.get("type"):
            if on_progress:
                on_progress(event)
            if event.get("type") == "done":
                done = event
    stderr = proc.stderr.read() if proc.stderr else ""
    code = proc.wait()
    if code != 0:
        raise OmnisnapError(
            f"omnisnap {' '.join(args)} exited {code}: {stderr.strip()}",
            code=code,
            stderr=stderr,
            args=args,
        )
    return {"code": code, "stderr": stderr, "done": done}


def _job_of(**opts: Any) -> dict[str, Any]:
    job: dict[str, Any] = {"pages": []}
    for page in opts.get("pages") or []:
        job["pages"].append({"url": page} if isinstance(page, str) else dict(page))
    if opts.get("engines"):
        job["engines"] = list(opts["engines"])
    for key in ("baseline", "runs", "viewport", "timeouts", "probes", "font_parity"):
        value = opts.get(key)
        if value is None:
            continue
        job["fontParity" if key == "font_parity" else key] = value
    return job


def run(
    pages: Iterable[str | dict[str, Any]],
    *,
    engines: Iterable[str] | None = None,
    baseline: str | None = None,
    runs: int | None = None,
    out: str = "./omnisnap-out",
    viewport: dict[str, Any] | None = None,
    timeouts: dict[str, Any] | None = None,
    probes: bool | None = None,
    font_parity: bool | None = None,
    on_progress: Callable[[dict[str, Any]], None] | None = None,
) -> dict[str, Any]:
    """Capture and compare a job; returns the parsed summary.json."""
    pages = list(pages)
    if not pages:
        raise OmnisnapError("run() needs at least one page")
    destination = Path(out).resolve()
    destination.mkdir(parents=True, exist_ok=True)
    job_path = destination / "omnisnap-job.json"
    job_path.write_text(json.dumps(
        _job_of(pages=pages, engines=engines, baseline=baseline, runs=runs, viewport=viewport,
                timeouts=timeouts, probes=probes, font_parity=font_parity),
        indent=1,
    ), encoding="utf-8")
    run_binary(["run", str(job_path), "--json", "--out", str(destination)], on_progress)
    return json.loads((destination / "summary.json").read_text(encoding="utf-8"))


def browsers() -> list[dict[str, Any]]:
    """The installed browsers, as `omnisnap browsers --json`."""
    result = run_binary(["browsers", "--json"])
    # browsers writes one JSON array, not NDJSON: read it from the process.
    # run_binary consumes stdout line by line, so re-run capturing it.
    proc = subprocess.run([resolve_binary(), "browsers", "--json"], capture_output=True, text=True, check=False)
    if proc.returncode != 0:
        raise OmnisnapError(f"omnisnap browsers exited {proc.returncode}: {proc.stderr.strip()}", code=proc.returncode, stderr=proc.stderr)
    return json.loads(proc.stdout)


def merge(bundles: Iterable[str], out: str, *, on_progress: Callable[[dict[str, Any]], None] | None = None) -> dict[str, Any]:
    """Merge bundle directories; returns the merged summary."""
    bundles = [str(Path(b).resolve()) for b in bundles]
    if not bundles:
        raise OmnisnapError("merge() needs bundle directories")
    destination = Path(out).resolve()
    run_binary(["merge", *bundles, "-o", str(destination)], on_progress)
    return json.loads((destination / "summary.json").read_text(encoding="utf-8"))


def report(bundle: str) -> str:
    """Re-render a bundle's report.html; returns its path."""
    directory = Path(bundle).resolve()
    run_binary(["report", str(directory)])
    return str(directory / "report.html")


__all__ = ["OmnisnapError", "run", "browsers", "merge", "report", "run_binary", "resolve_binary"]
