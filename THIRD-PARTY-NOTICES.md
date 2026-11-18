# Third-party notices

Omnisnap is licensed under the [Apache License 2.0](LICENSE.txt). The files
below are **development tooling**, not part of the shipped binary or the
packages, and keep their own licenses.

## Agent skills

Two sets of agent skills are vendored so that a contributor's tooling works out
of the box. They are plain Markdown instruction files, duplicated for the two
tool directories (`.agents/skills/` for OpenCode, `.claude/skills/` for Claude
Code). `skills-lock.json` pins the first set to its source commits and hashes.

### addyosmani/agent-skills

- Source: <https://github.com/addyosmani/agent-skills>
- Commit: `39b723ceebf81474d0557c1fecd01eddab585563`
- Paths: `.agents/skills/<name>/`, `.claude/skills/<name>/` for the 25 skills
  listed in `skills-lock.json`
- Modifications: none

```
MIT License

Copyright (c) 2025 Addy Osmani

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

### OpenSpec skills

- Source: <https://github.com/Fission-AI/OpenSpec>
- Paths: `.agents/skills/openspec-*/`, `.claude/skills/openspec-*/` (names
  starting with `openspec-`, installed by the OpenSpec CLI)
- Modifications: none

```
MIT License

Copyright (c) 2025 FISSION AI INC

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```
