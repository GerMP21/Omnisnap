# Omnisnap

## Git

- In a Claude Code cloud session (the environment has `CLAUDE_CODE_REMOTE`
  set), commit and push freely: commit finished work and push it to the
  session's working branch without asking first. Never force-push, rewrite
  history that has been pushed, or push to `main`.
- Everywhere else, never commit, amend, push, or open a pull request. Leave
  every change uncommitted in the working tree; the owner commits and pushes
  them. Do not treat an earlier "push it" or "commit it" as standing
  permission; this rule applies unless the owner asks for that specific action
  in the message being handled.
- Opening a pull request still needs the owner's ask, in a cloud session too.
- When a task is finished, give the owner:
  - a ready-to-use commit message for it (subject, blank line, body), unless it
    was already committed, and
  - when a branch or pull request is part of the work, the PR title and body.

## History (work journal)

The History MCP is the project's work journal; Omnisnap's project key there is
`ZANZIBAR`. Register in the same session as the work, not afterwards:

- **Every feature** (a backlog item, an OpenSpec change, a task in
  `tasks/todo.md`): `search` first so it is not a duplicate, then
  `create_feature`. Move it with `change_feature_status` as it is picked up,
  blocked, finished or dropped (blocked and dropped carry the reason).
- **Every decision** (the owner's calls and any design choice made between
  alternatives): `propose_decision` with context, the decision, the
  alternatives and the consequences, linked to its features. Only the owner
  accepts it, on the web.
- **Every finished unit of work, and work that was not done:** `log_work` with
  the outcome, the reason and the evidence (`GerMP21/Omnisnap` plus the commit
  hash once there is one; otherwise the files), linked to its features and
  decisions.
- Before saying something was or was not done, `search` History (and the git
  log); an empty result means "no record", not "not done".
- Fix a wrong entry with `correct_entry`, never with a second entry.
- When the History MCP is not connected, end the task with the list of
  features, decisions and work that still need registering.

## Developer wiki

- `docs/internals/` is the developer wiki; `.github/workflows/wiki.yml`
  publishes it to the GitHub Wiki when `main` changes. Never edit the GitHub
  Wiki directly.
- Every change updates, in the same change, every page there that describes
  what it touched (rule 7 in `CONTRIBUTING.md`): `grep -rl '<path>'
  docs/internals/` finds them. New files go in a page's `## Source files`.
- `go test ./test/internals/` must pass.
