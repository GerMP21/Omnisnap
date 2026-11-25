"""Tests for tools/site/build.py (specs/SPEC-website.md). No network: the docs
renderer is replaced by one that returns a saved GitHub response.

python3 -m unittest discover tools/site
"""

import contextlib
import io
import json
import pathlib
import re
import tempfile
import unittest

import build

HERE = pathlib.Path(__file__).resolve().parent
FIXTURE = (HERE / "testdata" / "github-action-setup.html").read_text()
TAG = "v0.1.0-rc.7"


def fake_renderer(path: str) -> str:
    """Every guide renders as the saved response for the setup guide."""
    return FIXTURE


def failing_renderer(path: str) -> str:
    raise build.BuildError(f"rendering {path} at {TAG}: GitHub answered 502 Bad Gateway")


def bundle(root: pathlib.Path, statuses: list[str]) -> pathlib.Path:
    """A merged bundle with one result per status, plus files the site must not publish."""
    merged = root / "merged"
    merged.mkdir()
    (merged / "report.html").write_text("<!doctype html><title>report</title>")
    results = []
    for i, status in enumerate(statuses):
        host = f"host{i}"
        shots = None
        if status not in build.NOT_COMPARED:
            for side in ("chrome", "firefox"):
                shot = merged / host / "home" / f"{side}-a.png"
                shot.parent.mkdir(parents=True, exist_ok=True)
                shot.write_bytes(b"\x89PNG")
            shots = {
                "baseline": {"src": f"{host}/home/chrome-a.png", "w": 1, "h": 1},
                "engine": {"src": f"{host}/home/firefox-a.png", "w": 1, "h": 1},
            }
        page_dir = merged / host / "home"
        page_dir.mkdir(parents=True, exist_ok=True)
        (page_dir / "chrome-a.json").write_text("{}")         # a capture
        (page_dir / "probes-firefox-a.json").write_text("{}")  # probes
        (page_dir / "findings.json").write_text("{}")         # findings
        r = {"status": status, "page": {"slug": "home"}}
        if shots:
            r["shots"] = shots
        results.append(r)
    (merged / "summary.json").write_text(json.dumps({"results": results}))
    return merged


class Versions(unittest.TestCase):
    def test_release_candidate(self):
        self.assertEqual(build.versions("v0.1.0-rc.7"),
                         {"version": "v0.1.0-rc.7", "npm_version": "0.1.0-rc.7", "pypi_version": "0.1.0rc7"})

    def test_stable(self):
        self.assertEqual(build.versions("v1.2.3")["pypi_version"], "1.2.3")

    def test_not_a_tag(self):
        for bad in ("main", "0.1.0", "v0.1", "v0.1.0-rc7"):
            with self.subTest(bad), self.assertRaises(build.BuildError):
                build.versions(bad)


class Links(unittest.TestCase):
    def rewrite(self, body, source="docs/github-action-setup.md"):
        return build.rewrite_links(body, source, TAG)

    def test_link_between_guides(self):
        out = self.rewrite('<a href="github-action-fork-comments.md">x</a>')
        self.assertIn('href="/docs/fork-comments/"', out)

    def test_link_between_guides_keeps_the_heading(self):
        out = self.rewrite('<a href="quickstart.md#install">x</a>')
        self.assertIn('href="/docs/quickstart/#install"', out)

    def test_repository_file_goes_to_github_at_the_tag(self):
        out = self.rewrite('<a href="../action/README.md">x</a>')
        self.assertIn(f'href="https://github.com/GerMP21/Omnisnap/blob/{TAG}/action/README.md"', out)

    def test_repository_directory_uses_tree(self):
        out = self.rewrite('<a href="../schema">x</a>')
        self.assertIn(f'href="https://github.com/GerMP21/Omnisnap/tree/{TAG}/schema"', out)

    def test_absolute_and_fragment_links_are_kept(self):
        body = '<a href="https://example.com/a.md">x</a><a href="#before-you-start">y</a><a href="mailto:a@b">z</a>'
        self.assertEqual(self.rewrite(body), body)

    def test_a_missing_file_fails_the_build(self):
        with self.assertRaisesRegex(build.BuildError, "does not exist"):
            self.rewrite('<a href="no-such-guide.md">x</a>')

    def test_a_link_out_of_the_repository_fails_the_build(self):
        with self.assertRaisesRegex(build.BuildError, "leaves the repository"):
            self.rewrite('<a href="../../outside.md">x</a>')

    def test_code_blocks_are_untouched(self):
        # GitHub escapes quotes in code, so href="…" there is text, not a link.
        body = '<pre><code>&lt;a href=&quot;quickstart.md&quot;&gt;</code></pre>'
        self.assertEqual(self.rewrite(body), body)

    def test_the_saved_response(self):
        out = self.rewrite(build.article(FIXTURE))
        self.assertIn('href="/docs/fork-comments/"', out)
        self.assertIn(f"/blob/{TAG}/action/README.md", out)
        self.assertIn(f"/blob/{TAG}/specs/SPEC-ci-action.md", out)
        # No relative .md link is left (absolute GitHub URLs are fine).
        self.assertNotRegex(out, r'<a\b[^>]*href="(?![a-z]+:)[^"#/][^"]*\.md')


class Anchors(unittest.TestCase):
    def test_prefix_is_removed(self):
        out = build.fix_anchors('<a id="user-content-before-you-start" class="anchor" href="#before-you-start">')
        self.assertIn('id="before-you-start"', out)

    def test_every_fragment_link_in_the_saved_response_has_a_target(self):
        body = build.fix_anchors(build.article(FIXTURE))
        ids = set(re.findall(r'\bid="([^"]+)"', body))
        targets = set(re.findall(r'href="#([^"]+)"', body))
        self.assertTrue(targets)
        self.assertEqual(targets - ids, set())
        self.assertNotIn("user-content-", body)


class Site(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        self.out = self.tmp / "_site"

    def build(self, render=fake_renderer, demo=None):
        return build.build(self.out, TAG, render, demo)

    def test_docs_pages_and_index(self):
        self.build()
        for slug in ("quickstart", "github-action", "fork-comments"):
            page = (self.out / "docs" / slug / "index.html").read_text()
            self.assertIn('<link rel="stylesheet" href="/site.css">', page)
            self.assertIn("<table>", page)
            self.assertIn('class="highlight', page)
            self.assertIn(f"/blob/{TAG}/", page)
        index = (self.out / "docs" / "index.html").read_text()
        for slug in ("quickstart", "github-action", "fork-comments"):
            self.assertIn(f'href="/docs/{slug}/"', index)

    def test_nav_on_every_page(self):
        self.build()
        for rel in ("index.html", "docs/index.html", "docs/quickstart/index.html", "schema/index.html", "404.html"):
            page = (self.out / rel).read_text()
            for href in ('href="/demo/"', 'href="/docs/"', 'href="/schema/"', 'href="https://github.com/GerMP21/Omnisnap"'):
                self.assertIn(href, page, f"{rel} misses {href}")

    def test_a_render_failure_stops_the_build(self):
        with self.assertRaisesRegex(build.BuildError, "502"):
            self.build(render=failing_renderer)

    def test_main_exits_non_zero_on_failure(self):
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            code = build.main(["--version", "not-a-tag", "--out", str(self.out), "--skip-docs"])
        self.assertEqual(code, 1)
        self.assertIn("not a release tag", err.getvalue())

    def test_landing_page_names_the_release(self):
        self.build(render=None)
        page = (self.out / "index.html").read_text()
        for v in ("v0.1.0-rc.7", "omnisnap@0.1.0-rc.7", "omnisnap==0.1.0rc7", "report.yml@v0.1.0-rc.7"):
            self.assertIn(v, page)
        self.assertNotIn("{{", page)
        for href in ('href="/demo/"', 'href="/docs/"', 'href="/schema/"'):
            self.assertIn(href, page)
        self.assertTrue((self.out / "site.css").is_file())

    def test_schemas_keep_their_urls_and_are_listed(self):
        self.build(render=None)
        listing = (self.out / "schema" / "index.html").read_text()
        for path in sorted((build.ROOT / "schema").glob("*.schema.json")):
            ident = json.loads(path.read_text())["$id"]
            rel = ident.split("omnisnap.org/", 1)[1]
            self.assertEqual((self.out / rel).read_text(), path.read_text())
            self.assertIn(f'href="/{rel}"', listing)

    def test_404_links_home(self):
        self.build(render=None)
        self.assertIn('href="/"', (self.out / "404.html").read_text())

    def test_a_rebuild_starts_clean(self):
        self.out.mkdir()
        (self.out / "stale.html").write_text("old")
        self.build(render=None)
        self.assertFalse((self.out / "stale.html").exists())


class Demo(unittest.TestCase):
    def setUp(self):
        self.tmp = pathlib.Path(tempfile.mkdtemp())
        self.out = self.tmp / "_site"

    def test_only_the_report_and_its_screenshots(self):
        merged = bundle(self.tmp, ["ok", "failed"])
        build.build(self.out, TAG, None, merged)
        published = sorted(str(p.relative_to(self.out / "demo")) for p in (self.out / "demo").rglob("*") if p.is_file())
        self.assertEqual(published, ["host0/home/chrome-a.png", "host0/home/firefox-a.png", "index.html"])

    def test_a_path_out_of_the_bundle_is_refused(self):
        merged = bundle(self.tmp, ["ok"])
        s = json.loads((merged / "summary.json").read_text())
        s["results"][0]["shots"]["engine"]["src"] = "../secret.png"
        (merged / "summary.json").write_text(json.dumps(s))
        with self.assertRaisesRegex(build.BuildError, "not inside the bundle"):
            build.build(self.out, TAG, None, merged)

    def test_compared_nothing(self):
        merged = bundle(self.tmp, ["failed", "unreachable"])
        self.assertFalse(build.compared_anything(merged))
        with self.assertRaisesRegex(build.BuildError, "compared nothing"):
            build.build(self.out, TAG, None, merged)
        self.assertFalse(self.out.exists(), "nothing may be written when the demo compared nothing")

    def test_no_summary_is_nothing_compared(self):
        empty = self.tmp / "merged"
        empty.mkdir()
        self.assertFalse(build.compared_anything(empty))
        self.assertFalse(build.compared_anything(self.tmp / "missing"))

    def test_one_comparison_among_failures_is_published(self):
        merged = bundle(self.tmp, ["failed", "ok", "unreachable"])
        self.assertTrue(build.compared_anything(merged))
        build.build(self.out, TAG, None, merged)
        self.assertTrue((self.out / "demo" / "index.html").is_file())

    def test_main_reports_compared_nothing(self):
        merged = bundle(self.tmp, ["unreachable"])
        err = io.StringIO()
        with contextlib.redirect_stderr(err):
            code = build.main(["--version", TAG, "--out", str(self.out), "--skip-docs", "--demo", str(merged)])
        self.assertEqual(code, 1)
        self.assertIn("the demo compared nothing", err.getvalue())


if __name__ == "__main__":
    unittest.main()
