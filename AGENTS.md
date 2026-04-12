# AGENTS.md

This file is the shared agent entrypoint for tools that support `AGENTS.md`.

In this repository, the canonical per-directory agent instructions live in stacked `CLAUDE.md` files. Use this file to discover and apply those `CLAUDE.md` instructions correctly.

## Instruction Loading

1. Always read the repository root `CLAUDE.md` first.
2. When working in a specific subtree, also read every `CLAUDE.md` on the directory path from the repo root down to the target directory.
3. Apply those files in order from general to specific. Example for work in `apps/mirrors/cpan/primary/`:
   - `<repo-root>/CLAUDE.md`
   - `<repo-root>/apps/mirrors/CLAUDE.md`
   - `<repo-root>/apps/mirrors/cpan/primary/CLAUDE.md`

## Scope And Precedence

- Treat the root `CLAUDE.md` as repo-wide defaults.
- Treat deeper `CLAUDE.md` files as additive and more specific to their subtree.
- If two applicable `CLAUDE.md` files conflict, the deepest applicable file wins.
- Apply subtree-specific guidance only within that subtree.

## Multi-File Work

- For a single-file task, load the `CLAUDE.md` stack for that file's directory.
- For multi-file tasks spanning different subtrees, load the applicable stack for each file you inspect or modify.
- Do not apply instructions from one subtree to unrelated sibling subtrees unless the root `CLAUDE.md` says to do so.
- If the task starts as repo-wide exploration and no target path is known yet, begin with the root `CLAUDE.md` and load deeper files as the task narrows.

## Canonical Files Only

- Use files named exactly `CLAUDE.md` as the authoritative stacked instructions.
- Ignore conflict copies, backups, and similarly named files such as `CLAUDE.sync-conflict-*.md` unless the user explicitly asks about them.

## Tool Configuration

- Tools that support custom context filenames should be configured to load `AGENTS.md` from the repository root.
- If a tool also supports another agent file such as `GEMINI.md`, that file should either repeat these rules or defer to `AGENTS.md`, but `CLAUDE.md` remains the canonical source of per-directory instructions.

## Ongoing Check

When you move into a new directory during exploration or implementation, check whether a more specific `CLAUDE.md` applies before proceeding.
