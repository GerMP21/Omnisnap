"""Task 84: the Python API against a stub binary."""
import json
import os
import stat
import sys
from pathlib import Path

import pytest

import omnisnap
from omnisnap import OmnisnapError, browsers, merge, report, run

STUB = r'''#!{python}
import json, os, sys
from pathlib import Path
args = sys.argv[1:]
out_index = args.index("--out") if "--out" in args else None
out = args[out_index + 1] if out_index is not None else "./omnisnap-out"
def progress(t, **kw):
    print(json.dumps({{"type": t, **kw}}), flush=True)
if args[0] == "browsers":
    print(json.dumps([{{"id": "chrome", "family": "blink", "protocol": "cdp", "executable": "/bin/chrome", "version": "157", "name": "Chrome", "source": "path"}}]))
    sys.exit(0)
if args[0] == "run":
    progress("job", pages=1)
    progress("pageStart", slug="home")
    progress("capture", slug="home", engine="chrome", run="a", nodes=42, ms=12)
    progress("pageDone", slug="home", status="ok")
    if os.environ.get("STUB_FAIL") == "1":
        sys.stderr.write("stub: capture failed\n")
        sys.exit(3)
    Path(out).mkdir(parents=True, exist_ok=True)
    summary = {{"$schema": "x", "tool": {{"name": "omnisnap", "version": "1.0.0"}},
               "generated": "2026-11-18T00:00:00Z", "job": {{}}, "hosts": [], "skipped": [], "pages": [], "results": []}}
    Path(out, "summary.json").write_text(json.dumps(summary))
    Path(out, "stub-args.json").write_text(json.dumps(args))
    progress("done", summary=str(Path(out, "summary.json")))
    sys.exit(0)
if args[0] == "merge":
    dest = Path(args[args.index("-o") + 1])
    dest.mkdir(parents=True, exist_ok=True)
    dest.joinpath("summary.json").write_text(json.dumps({{"tool": {{"name": "omnisnap", "version": "1.0.0"}}}}))
    sys.exit(0)
if args[0] == "report":
    Path(args[1], "report.html").write_text("<!doctype html>")
    sys.exit(0)
sys.exit(2)
'''


@pytest.fixture()
def stub(tmp_path, monkeypatch):
    script = tmp_path / "stub.py"
    script.write_text(STUB.format(python=sys.executable))
    script.chmod(script.stat().st_mode | stat.S_IEXEC)
    shim = tmp_path / ("omnisnap.cmd" if sys.platform.startswith("win") else "omnisnap")
    if sys.platform.startswith("win"):
        shim.write_text(f'@echo off\r\n"{sys.executable}" "{script}" %*\r\n')
    else:
        shim.write_text(f'#!/bin/sh\nexec "{sys.executable}" "{script}" "$@"\n')
        shim.chmod(0o755)
    monkeypatch.setenv("OMNISNAP_BINARY", str(shim))
    return shim


def test_run_streams_progress_in_order(stub, tmp_path):
    events = []
    summary = run(pages=["http://localhost:3000/"], engines=["chrome"], out=str(tmp_path / "out"),
                  on_progress=lambda e: events.append(e["type"]))
    assert summary["tool"]["name"] == "omnisnap"
    assert events == ["job", "pageStart", "capture", "pageDone", "done"]
    job = json.loads((tmp_path / "out" / "omnisnap-job.json").read_text())
    assert job["pages"] == [{"url": "http://localhost:3000/"}]
    assert job["engines"] == ["chrome"]


@pytest.mark.parametrize("allow, want", [(True, True), (False, False)])
def test_allow_small_pages_flag(stub, tmp_path, allow, want):
    out = tmp_path / f"out-{allow}"
    omnisnap.run(pages=["http://localhost:3000/"], out=str(out), allow_small_pages=allow)
    args = json.loads((out / "stub-args.json").read_text())
    assert ("--allow-small-pages" in args) == want, args


def test_nonzero_exit_is_typed(stub, tmp_path, monkeypatch):
    monkeypatch.setenv("STUB_FAIL", "1")
    with pytest.raises(OmnisnapError) as info:
        run(pages=["http://localhost:3000/"], out=str(tmp_path / "out"))
    assert info.value.code == 3
    assert "capture failed" in info.value.stderr


def test_browsers(stub):
    got = browsers()
    assert got[0]["id"] == "chrome"


def test_merge_and_report(stub, tmp_path):
    summary = merge(["/a", "/b"], str(tmp_path / "merged"))
    assert summary["tool"]["name"] == "omnisnap"
    bundle = tmp_path / "bundle"
    bundle.mkdir()
    html = report(str(bundle))
    assert Path(html).read_text().startswith("<!doctype")


def test_version_exposed():
    assert omnisnap.__version__ == "0.1.0rc5"


def test_types_cover_the_schema_required_fields():
    """Task 84: every field the schema requires appears in the TypedDicts."""
    root = Path(__file__).resolve().parents[4]
    schema = json.loads((root / "schema" / "summary.schema.json").read_text())
    source = (Path(omnisnap.__file__).parent / "types.py").read_text()

    def section(name):
        start = source.index(f"class {name}(")
        end = source.find("\nclass ", start + 1)
        return source[start: end if end > 0 else len(source)]

    for field in schema["required"]:
        # Python cannot spell "$schema": the attribute is `schema`.
        name = "schema" if field == "$schema" else field
        assert name in section("Summary"), f"Summary.{field}"
    for field in schema["$defs"]["result"]["required"]:
        assert field in section("Result"), f"Result.{field}"
    for field in schema["$defs"]["cause"]["required"]:
        assert field in section("Cause"), f"Cause.{field}"


def test_license_copy_matches_the_repository():
    """The wheel ships packages/pypi/LICENSE.txt (license-files): it must be
    the repository's LICENSE.txt, word for word."""
    pypi = Path(__file__).resolve().parents[2]
    root = pypi.parent.parent
    if not (root / "LICENSE.txt").is_file():
        pytest.skip("not in a repository checkout")
    assert (pypi / "LICENSE.txt").read_bytes() == (root / "LICENSE.txt").read_bytes()
