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
