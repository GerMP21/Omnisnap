# Omnisnap Action

Report-only cross-engine layout comparison on GitHub runners. One reusable
workflow (Linux, Windows, macOS, merged report, one PR comment) and this
composite action for custom matrices.

```yaml
on: pull_request
jobs:
  omnisnap:
    uses: GerMP21/Omnisnap/.github/workflows/report.yml@v1
    with:
      start: npm ci && npm run build && npx serve -l 3000 dist &
      wait-on: http://localhost:3000
      pages: |
        http://localhost:3000/
        http://localhost:3000/pricing
```

The job never fails the build. Fork PRs get their comment from a `workflow_run`
follow-up that never runs fork code. macOS runs on every PR by default; pass
`macos: labelled` to run Safari only on PRs labelled `omnisnap:safari`.
