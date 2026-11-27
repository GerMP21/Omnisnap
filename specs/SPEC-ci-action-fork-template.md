# Spec Delta: fork-comment template and its guide

Status: implemented (2026-11-18)

Original: [SPEC-ci-action.md](SPEC-ci-action.md) §"Permissions and forks". Only
the areas below change; everything else inherits.

## Objective

Give consumers the fork-safe comment workflow in two forms — a detailed guide
and a copy-paste template — without keeping two copies of the YAML in the
tree.

## Current behavior

- `.github/workflows/report-comment.yml` is a reusable workflow
  (`on: workflow_call`), added today.
- `action/fork-comment.yml` is the consumer template, sitting beside
  `action/action.yml`.
- `action/README.md` has a short "Pull requests from forks" section with the
  default-branch rule, the three-level limit and the artifact download.
- There is no page in `docs/` that a consumer can be sent to.

## New behavior

1. **Template**: `.github/workflow-templates/omnisnap-fork-comment.yml` plus
   `omnisnap-fork-comment.properties.json` (`name`, `description`,
   `iconName`, `categories`), following GitHub's starter-workflow layout.
   `action/fork-comment.yml` is deleted so exactly one copy of the YAML exists.
2. **Guide**: `docs/github-action-fork-comments.md` — why a fork's
   `pull_request` run gets a read-only token, the default-branch requirement
   for `workflow_run`, the write-token property of the follow-up, the
   three-level chaining limit, the cross-run artifact download (`run-id` +
   `github-token`), the permissions the caller must grant, the exact copy
   steps, and what to do when the comment does not appear.
3. **Pointer**: `action/README.md` keeps a two-line summary that links to both;
   it no longer embeds the YAML.
4. **Caveat, documented in both places**: GitHub's template UI reads
   `workflow-templates/` from an **organization's** `.github` repository
   ([creating workflow templates](https://docs.github.com/en/actions/how-tos/reuse-automations/create-workflow-templates)).
   Inside this project repository the folder is inert: it is a versioned,
   discoverable copy, not a template GitHub offers in the UI. If Omnisnap later
   has an org-level `.github` repository, the same two files become a real
   template unchanged.

## Success criteria

1. Exactly one workflow YAML for the fork comment exists in the repository,
   under `.github/workflow-templates/`, with a matching `.properties.json`.
2. `docs/github-action-fork-comments.md` contains the full workflow and the
   four doc-cited rules; `action/README.md` links to it.
3. Nothing under `.github/workflow-templates/` runs: GitHub only executes
   `.github/workflows/`, and a `grep` of the workflows shows no reference.
4. The YAML parses (the existing `yamlcheck`-style check) and matches
   `report-comment.yml`'s inputs (`run-id`, `pr`, `branch`, `artifact`).

## Boundaries

- Always: cite docs.github.com for the token, default-branch and chaining
  rules; keep the template free of secrets and of `actions/checkout`.
- Ask first: renaming `report-comment.yml`'s inputs or its `uses:` ref
  (`@v1`), which the template pins.
- Never: place the template in `.github/workflows/` (it would run on every
  `workflow_run`), or have the follow-up checkout the repository.

## Out of scope (deferred)

- Publishing to the `actions/starter-workflows` repository or the Marketplace.
- A JavaScript Action (the composite action plus workflows stay the only
  shipping mechanism).
- Running the Action on a sample repository (Tasks 79–81).

## Open questions

- The template pins `GerMP21/Omnisnap/.github/workflows/report-comment.yml@v0.2.0-rc.2`.
  Follow the release tag cadence (v1, v2, …) or pin a commit SHA in the
  template and move it per release?
- Should the guide live at `docs/github-action-fork-comments.md` or be folded
  into `docs/quickstart.md` as a section? The delta proposes a page of its
  own because the failure mode ("no comment on a fork PR") needs a place to
  search for.
