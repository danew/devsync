# Architecture

DevSync has one authority model: Git history determines synchronization safety.

## Components

- `workspace`: repository discovery, config resolution, convention mapping.
- `git`: local and remote Git inspection. DevSync does not mutate Git history during sync.
- `ssh`: remote command execution through the standard SSH client.
- `mutagen`: session discovery, creation, flushing, health normalization, drift detection.
- `plan`: explicit operation planning for dry-run and sync execution.
- `lock`: local lockfiles to prevent concurrent orchestration.
- `syncstate`: local metadata for last successful flush visibility.

## Sync Flow

1. Discover Git repository root with `git rev-parse --show-toplevel`.
2. Resolve normalized configuration.
3. Inspect local and remote Git state.
4. Abort on detached HEAD, branch mismatch, unknown ancestry, or any unequal local/remote history.
5. Acquire local workspace lock.
6. Discover or create Mutagen session.
7. Abort on session drift unless the user explicitly recreates it.
8. Resume if paused, flush, verify health, pause the session, record last successful flush.

## Git Boundary

DevSync treats the remote workspace repository as a peer working clone, not as canonical Git transport authority.

Git history belongs to the repository's canonical remotes such as `origin`. Working tree synchronization belongs to Mutagen. The remote workspace is only the other endpoint for filesystem synchronization.

DevSync v1 therefore does not run automatic `git pull` or `git push` during `devsync sync`. If local and remote workspace HEADs differ, DevSync aborts with remediation guidance. Operators must synchronize Git history explicitly with their normal upstream remotes before running filesystem sync.

Peer-clone pulls are unsafe because `git pull <peer-workspace-path> <branch>` writes `FETCH_HEAD` from the peer clone and can fast-forward the local branch while leaving `origin/<branch>` unchanged. The result is a local branch that appears ahead of its canonical upstream even though the mutation came from the peer workspace transport.

## Session Boundary

DevSync uses Mutagen as a synchronization engine, not as always-on replication infrastructure.

`devsync sync` is one-shot. It creates or resumes the resolved Mutagen session, flushes until synchronization has settled, then pauses the session before exiting. This protects branch switching and local experiments from background propagation outside explicit operator intent.

Continuous synchronization is opt-in:

- `devsync attach` creates or resumes the session and leaves it active.
- `devsync detach` pauses the session and stops background propagation.

Status output reports whether the workspace is in one-shot mode or attached continuous mode.

## Non-Goals

DevSync v1 intentionally excludes migration, daemons, distributed sync, service forwarding, tmux orchestration, TypeScript runtime config, automatic merge/rebase, and automatic branch switching.
