#!/usr/bin/env python3
"""Build omnisnap.org (specs/SPEC-website.md) into _site/ for Cloudflare Workers.

The site is built from one release tag:

- /                  site/index.html, with the release's version substituted;
- /docs/<slug>/      the three user guides, rendered as HTML by GitHub's API
                     from the tag (the contents endpoint with
                     Accept: application/vnd.github.html), with their links
                     rewritten for the site;
- /demo/             the report.html of a merged bundle and only the
                     screenshots it references (--demo);
- /schema/           the schema list, and each schema at the path its $id
                     names (https://omnisnap.org/schema/<name>/<major>.json);
- /404.html          the not-found page.

Only the standard library is used. wrangler.json uploads the output as the
Worker's static assets; .github/workflows/deploy.yml runs this for a release.

usage: python3 tools/site/build.py --version <tag> [--demo <merged-dir>]
                                   [--out _site] [--skip-docs]

GITHUB_TOKEN (or GH_TOKEN) authenticates the docs requests. --skip-docs builds
without them, for a local preview only. With --demo, a bundle that compared
nothing (every result failed or unreachable, or no summary.json) is an error:
the deploy stops and the live site keeps serving the previous release.
"""

from __future__ import annotations

import argparse
import glob
import html
import json
import os
import pathlib
import posixpath
import re
import shutil
import sys
import urllib.error
import urllib.request
from typing import Callable

ROOT = pathlib.Path(__file__).resolve().parents[2]
REPO = "GerMP21/Omnisnap"
GITHUB = f"https://github.com/{REPO}"

# The guides published under /docs/: (slug, source path, title).
GUIDES = [
    ("quickstart", "docs/quickstart.md", "Quick start"),
    ("github-action", "docs/github-action-setup.md", "The GitHub Action"),
    ("fork-comments", "docs/github-action-fork-comments.md", "Comments on pull requests from forks"),
]

# Result statuses that mean nothing was compared (internal/schema/progress.go).
NOT_COMPARED = {"failed", "unreachable"}


class BuildError(Exception):
    """A problem that must stop the build, and so the deploy."""


# Versions ------------------------------------------------------------------

_TAG = re.compile(r"^v(\d+\.\d+\.\d+)(?:-(alpha|beta|rc)\.(\d+))?$")
_PEP440 = {"alpha": "a", "beta": "b", "rc": "rc"}


def versions(tag: str) -> dict[str, str]:
    """The three spellings of a release: the tag, npm's and PyPI's (PEP 440)."""
    m = _TAG.match(tag)
    if not m:
        raise BuildError(f"--version {tag!r} is not a release tag like v0.1.0 or v0.1.0-rc.7")
    base, pre, n = m.groups()
    npm = base + (f"-{pre}.{n}" if pre else "")
    pypi = base + (f"{_PEP440[pre]}{n}" if pre else "")
    return {"version": tag, "npm_version": npm, "pypi_version": pypi}


# Page template ---------------------------------------------------------------

NAV = [("/demo/", "Demo"), ("/docs/", "Docs"), ("/schema/", "Schemas"), (GITHUB, "GitHub")]


def page(title: str, body: str, version: str = "", active: str = "", main_class: str = "") -> str:
    """A full HTML page with the shared header, nav and footer."""
    links = []
    for href, label in NAV:
        current = ' aria-current="page"' if href == active else ""
        links.append(f'<a href="{href}"{current}>{label}</a>')
    full_title = "Omnisnap" if title == "Omnisnap" else f"{title} · Omnisnap"
    footer_version = f" {html.escape(version)}" if version else ""
    cls = f' class="{main_class}"' if main_class else ""
    return (
        "<!doctype html>\n"
        '<html lang="en">\n<head>\n<meta charset="utf-8">\n'
        '<meta name="viewport" content="width=device-width, initial-scale=1">\n'
        f"<title>{html.escape(full_title)}</title>\n"
        '<meta name="description" content="Element-level cross-browser layout diffs from the browsers you already have.">\n'
        '<link rel="stylesheet" href="/site.css">\n</head>\n<body>\n'
        '<header class="site-header"><a class="brand" href="/">Omnisnap</a>\n'
        f'<nav aria-label="Site">{" ".join(links)}</nav></header>\n'
        f"<main{cls}>\n{body}\n</main>\n"
        f'<footer class="site-footer">Omnisnap{footer_version} · '
        f'<a href="{GITHUB}">GitHub</a> · '
        f'<a href="{GITHUB}/blob/main/LICENSE.txt">AGPL-3.0</a></footer>\n'
        "</body>\n</html>\n"
    )


# Docs ------------------------------------------------------------------------

Renderer = Callable[[str], str]


def github_renderer(ref: str, token: str | None) -> Renderer:
    """Fetch a repository file at ref, rendered as HTML the way github.com shows it."""

    def render(path: str) -> str:
        url = f"https://api.github.com/repos/{REPO}/contents/{path}?ref={ref}"
        req = urllib.request.Request(url, headers={
            "Accept": "application/vnd.github.html",
            "X-GitHub-Api-Version": "2022-11-28",
            "User-Agent": "omnisnap-site-build",
        })
        if token:
            req.add_header("Authorization", f"Bearer {token}")
        try:
            with urllib.request.urlopen(req, timeout=60) as res:
                return res.read().decode("utf-8")
        except urllib.error.HTTPError as e:
            raise BuildError(f"rendering {path} at {ref}: GitHub answered {e.code} {e.reason}") from e
        except urllib.error.URLError as e:
            raise BuildError(f"rendering {path} at {ref}: {e.reason}") from e

    return render


_ARTICLE = re.compile(r"<article\b[^>]*>(.*)</article>", re.S)


def article(rendered: str) -> str:
    """The rendered file without GitHub's wrapper (<div id="file"><article>)."""
    m = _ARTICLE.search(rendered)
    return m.group(1) if m else rendered


# Only <a> tags: a literal href="…" inside a code block is escaped or split
# into highlight spans by GitHub, so it never matches.
_HREF = re.compile(r'(<a\b[^>]*?\bhref=")([^"]*)(")')
_SCHEME = re.compile(r"^[a-zA-Z][a-zA-Z0-9+.-]*:")


def rewrite_links(body: str, source: str, ref: str, root: pathlib.Path = ROOT) -> str:
    """Point a guide's links at the site (other guides) or at GitHub at ref.

    A relative link to a file that does not exist at ref is an error: it would
    be a broken link on the site.
    """
    guides = {src: slug for slug, src, _ in GUIDES}
    problems = []

    def one(m: re.Match) -> str:
        target = html.unescape(m.group(2))
        if not target or target.startswith("#") or target.startswith("//") or _SCHEME.match(target):
            return m.group(0)
        path, _, fragment = target.partition("#")
        frag = f"#{fragment}" if fragment else ""
        if path.startswith("/"):
            resolved = posixpath.normpath(path.lstrip("/"))
        else:
            resolved = posixpath.normpath(posixpath.join(posixpath.dirname(source), path))
        if resolved.startswith("../") or resolved == "..":
            problems.append(f"{source}: link {target!r} leaves the repository")
            return m.group(0)
        if resolved in guides:
            new = f"/docs/{guides[resolved]}/{frag}"
        else:
            local = root / resolved
            if not local.exists():
                problems.append(f"{source}: link {target!r} names {resolved}, which does not exist")
                return m.group(0)
            kind = "tree" if local.is_dir() else "blob"
            new = f"{GITHUB}/{kind}/{ref}/{resolved}{frag}"
        return m.group(1) + html.escape(new, quote=True) + m.group(3)

    out = _HREF.sub(one, body)
    if problems:
        raise BuildError("; ".join(problems))
    return out


_USER_CONTENT = re.compile(r'\b(id|name)="user-content-')


def fix_anchors(body: str) -> str:
    """Drop GitHub's user-content- prefix so plain #heading links work.

    github.com maps #x to id="user-content-x" with JavaScript; the site has no
    script, so the ids carry the plain slug instead.
    """
    return _USER_CONTENT.sub(r'\1="', body)


def build_docs(out: pathlib.Path, render: Renderer, ref: str, version: str, root: pathlib.Path = ROOT) -> None:
    items = []
    for slug, src, title in GUIDES:
        body = fix_anchors(rewrite_links(article(render(src)), src, ref, root))
        edit = f'<p class="doc-source">Source: <a href="{GITHUB}/blob/{ref}/{src}"><code>{src}</code></a> at {html.escape(ref)}</p>'
        doc = f'<article class="markdown-body">\n{body}\n</article>\n{edit}'
        write(out / "docs" / slug / "index.html", page(title, doc, version, "/docs/", "doc"))
        items.append(f'<li><a href="/docs/{slug}/">{html.escape(title)}</a></li>')
    index = (
        "<h1>Docs</h1>\n"
        "<p>The user guides, as of this release. The full README, the command reference and the "
        f'developer wiki are on <a href="{GITHUB}">GitHub</a>.</p>\n'
        '<ul class="doc-list">\n' + "\n".join(items) + "\n</ul>"
    )
    write(out / "docs" / "index.html", page("Docs", index, version, "/docs/", "doc"))


# Landing page ----------------------------------------------------------------

_PLACEHOLDER = re.compile(r"\{\{(\w+)\}\}")


def build_landing(out: pathlib.Path, vers: dict[str, str], root: pathlib.Path = ROOT) -> None:
    src = (root / "site" / "index.html").read_text()

    def sub(m: re.Match) -> str:
        if m.group(1) not in vers:
            raise BuildError(f"site/index.html: unknown placeholder {m.group(0)}")
        return vers[m.group(1)]

    body = _PLACEHOLDER.sub(sub, src)
    write(out / "index.html", page("Omnisnap", body, vers["version"], "", "home"))
    shutil.copyfile(root / "site" / "site.css", out / "site.css")


# Schemas ---------------------------------------------------------------------

def build_schemas(out: pathlib.Path, version: str, root: pathlib.Path = ROOT) -> int:
    paths = sorted(glob.glob(str(root / "schema" / "*.schema.json")))
    if not paths:
        raise BuildError("no schema/*.schema.json found")
    published = []
    for path in paths:
        text = pathlib.Path(path).read_text()
        doc = json.loads(text)
        ident = doc.get("$id", "")
        marker = "omnisnap.org/"
        if marker not in ident:
            raise BuildError(f"{path}: $id is not on omnisnap.org")
        rel = ident.split(marker, 1)[1]
        write(out / rel, text)
        published.append((rel, doc.get("title", rel)))
    rows = "\n".join(
        f'<li><a href="/{rel}">{html.escape(title)}</a> <code>/{rel}</code></li>'
        for rel, title in published
    )
    body = (
        "<h1>JSON Schemas</h1>\n"
        "<p>The contracts of Omnisnap's job file and bundle. Every document Omnisnap writes names "
        "its schema in <code>$schema</code>, and each schema is published at the URL its "
        "<code>$id</code> names. Changes are additive within a major version.</p>\n"
        f'<ul class="schema-list">\n{rows}\n</ul>\n'
        f'<p>Specification: <a href="{GITHUB}/blob/main/specs/SPEC-bundle-schema.md">SPEC-bundle-schema.md</a>.</p>'
    )
    write(out / "schema" / "index.html", page("JSON Schemas", body, version, "/schema/"))
    return len(published)


def build_404(out: pathlib.Path, version: str) -> None:
    body = "<h1>Not found</h1>\n<p>No such page or schema. Start from <a href=\"/\">the home page</a>.</p>"
    write(out / "404.html", page("Not found", body, version))


# Demo ------------------------------------------------------------------------

def _summary(merged: pathlib.Path) -> dict | None:
    path = merged / "summary.json"
    if not path.is_file():
        return None
    return json.loads(path.read_text())


def compared_anything(merged: pathlib.Path) -> bool:
    """Whether the merged bundle holds at least one comparison."""
    s = _summary(merged)
    if s is None:
        return False
    return any(r.get("status") not in NOT_COMPARED for r in s.get("results") or [])


def _safe(rel: str) -> str:
    """A bundle-relative path, refused if it could reach outside the bundle."""
    parts = rel.replace("\\", "/").split("/")
    if not rel or rel.startswith("/") or "\\" in rel or re.match(r"^[a-zA-Z]:", rel) or ".." in parts:
        raise BuildError(f"demo: screenshot path {rel!r} is not inside the bundle")
    return "/".join(p for p in parts if p not in ("", "."))


def build_demo(out: pathlib.Path, merged: pathlib.Path) -> int:
    """Copy the report and only the screenshots it references; return their count."""
    if not compared_anything(merged):
        raise BuildError("the demo compared nothing: every capture failed or no bundle was produced")
    s = _summary(merged)
    report = merged / "report.html"
    if not report.is_file():
        raise BuildError(f"demo: {report} is missing")
    demo = out / "demo"
    demo.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(report, demo / "index.html")
    shots = set()
    for r in s.get("results") or []:
        for side in ("baseline", "engine"):
            src = ((r.get("shots") or {}).get(side) or {}).get("src")
            if src:
                shots.add(_safe(src))
    for rel in sorted(shots):
        source = merged / rel
        if not source.is_file():
            raise BuildError(f"demo: the report references {rel}, which the bundle does not hold")
        target = demo / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, target)
    return len(shots)


# Build -----------------------------------------------------------------------

def write(path: pathlib.Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def build(out: pathlib.Path, tag: str, render: Renderer | None, demo: pathlib.Path | None,
          root: pathlib.Path = ROOT) -> str:
    """Build the whole site into out; return a one-line summary."""
    vers = versions(tag)
    if demo is not None and not compared_anything(demo):
        # Checked before anything is written or fetched: this stops the deploy.
        raise BuildError("the demo compared nothing: every capture failed or no bundle was produced")
    if out.exists():
        shutil.rmtree(out)
    out.mkdir(parents=True)
    build_landing(out, vers, root)
    schemas = build_schemas(out, tag, root)
    build_404(out, tag)
    parts = [f"{schemas} schema(s)"]
    if render is not None:
        build_docs(out, render, tag, tag, root)
        parts.append(f"{len(GUIDES)} guide(s)")
    if demo is not None:
        shots = build_demo(out, demo)
        parts.append(f"demo with {shots} screenshot(s)")
    return ", ".join(parts)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Build omnisnap.org into a directory.")
    parser.add_argument("--version", required=True, help="the release tag, e.g. v0.1.0-rc.7")
    parser.add_argument("--out", default=str(ROOT / "_site"))
    parser.add_argument("--demo", help="a merged bundle directory to publish at /demo/")
    parser.add_argument("--skip-docs", action="store_true",
                        help="build without the docs (local preview only; the deploy never passes it)")
    args = parser.parse_args(argv)
    token = os.environ.get("GITHUB_TOKEN") or os.environ.get("GH_TOKEN")
    render = None if args.skip_docs else github_renderer(args.version, token)
    try:
        summary = build(pathlib.Path(args.out), args.version,
                        render, pathlib.Path(args.demo) if args.demo else None)
    except BuildError as e:
        print(f"tools/site: {e}", file=sys.stderr)
        return 1
    print(f"tools/site: {summary} in {args.out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
