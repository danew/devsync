# Architecture

DevSync has one authority model: Git history determines synchronization safety.

## Components

- `workspace`: repository discovery, config resolution, convention mapping.
- `git`: local and remote Git inspection plus guarded push/pull operations.
- `ssh`: remote command execution through the standard SSH client.
- `mutagen`: session discovery, creation, flushing, health normalization, drift detection.
- `plan`: explicit operation planning for dry-run and sync execution.
- `lock`: local lockfiles to prevent concurrent orchestration.
- `syncstate`: local metadata for last successful flush visibility.

## Sync Flow

1. Discover Git repository root with `git rev-parse --show-toplevel`.
2. Resolve normalized configuration.
3. Inspect local and remote Git state.
4. Abort on detached HEAD, branch mismatch, unknown ancestry, or divergence.
5. Acquire local workspace lock.
6. Push or pull only when ancestry says it is safe.
7. Discover or create Mutagen session.
8. Abort on session drift unless the user explicitly recreates it.
9. Resume if paused, flush, verify health, record last successful flush.

## Non-Goals

DevSync v1 intentionally excludes migration, daemons, distributed sync, service forwarding, tmux orchestration, TypeScript runtime config, automatic merge/rebase, and automatic branch switching.
