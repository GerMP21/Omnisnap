#!/usr/bin/env python3
"""Build the schema site for Cloudflare Workers.

Each schema is published at the path its $id claims
(https://omnisnap.org/schema/<name>/<major>.json), so the $id stays the single
source of truth and the file name is free to change. The output, _site/, is
what wrangler.json uploads as static assets.

usage: python3 tools/site/build.py [--out _site]
"""

import argparse
import glob
import json
import pathlib
import shutil
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]


def build(out: pathlib.Path) -> int:
    if out.exists():
        shutil.rmtree(out)
    paths = sorted(glob.glob(str(ROOT / "schema" / "*.schema.json")))
    if not paths:
        sys.exit("tools/site: no schema/*.schema.json found")
    published = []
    for path in paths:
        doc = json.loads(pathlib.Path(path).read_text())
        ident = doc.get("$id", "")
        marker = "omnisnap.org/"
        if marker not in ident:
            sys.exit(f"tools/site: {path}: $id is not on omnisnap.org")
        rel = ident.split(marker, 1)[1]
        target = out / rel
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(pathlib.Path(path).read_text())
        published.append((rel, doc.get("title", rel)))
    rows = "\n".join(
        f'    <li><a href="/{rel}">{title}</a> <code>/{rel}</code></li>'
        for rel, title in published
    )
    (out / "index.html").write_text(
        "<!doctype html>\n"
        '<meta charset="utf-8">\n'
        "<title>Omnisnap schemas</title>\n"
        "<h1>Omnisnap JSON Schemas</h1>\n"
        "<p>The contracts of <code>specs/SPEC-bundle-schema.md</code>.</p>\n"
        "<ul>\n" + rows + "\n</ul>\n"
    )
    (out / "404.html").write_text(
        "<!doctype html>\n"
        '<meta charset="utf-8">\n'
        "<title>Not found</title>\n"
        "<h1>404</h1>\n"
        '<p>No such page or schema. See <a href="/">the index</a>.</p>\n'
    )
    (out / ".nojekyll").write_text("")
    return len(published)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default=str(ROOT / "_site"))
    args = parser.parse_args()
    out = pathlib.Path(args.out)
    count = build(out)
    print(f"tools/site: {count} schema(s) in {out}")


if __name__ == "__main__":
    main()
