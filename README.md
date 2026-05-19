# DevSync

DevSync is a Git-aware workspace synchronization tool for remote-first software development.

It composes Git, SSH, and Mutagen. It does not replace them.

## What It Is

- A lightweight CLI orchestration layer around Git, SSH, and Mutagen.
- Convention-first workspace sync for local macOS to remote Linux development.
- Fail-closed synchronization: ambiguous state aborts rather than guessing.
- Transparent: repositories, SSH, and Mutagen sessions remain inspectable with native tools.

## What It Is Not

- Not a Git replacement.
- Not Dropbox for code.
- Not a distributed filesystem.
- Not a daemon.
- Not an automatic conflict resolver.
- Not a workspace migration or node orchestration platform.

## Quick Start

```bash
make build
./bin/devsync version
./bin/devsync bootstrap
./bin/devsync bootstrap --init-workspace
./bin/devsync doctor
./bin/devsync sync --dry-run
./bin/devsync sync
```

## Core Guarantees

- `.git` is never synchronized.
- Branch names must match.
- Divergence always aborts.
- Git history is authoritative.
- Filesystem sync happens only after Git validation succeeds.
- DevSync never silently recreates Mutagen sessions.

## Configuration

Global config lives at:

```text
~/.config/devsync/config.yaml
```

Workspace overrides live at the repository root:

```text
.devsync.yaml
```

The default convention maps:

```text
~/remote/work/example -> core-dev:~/workspace/work/example
```

See `docs/examples.md` for configuration examples.

## Operational Commands

- `devsync status`: inspect Git, config, Mutagen, and sync freshness.
- `devsync sync --dry-run`: show the operation plan without mutation.
- `devsync sync`: validate Git, reconcile session state, flush Mutagen.
- `devsync doctor`: validate local and remote prerequisites.
- `devsync session inspect`: inspect the resolved Mutagen session.
- `devsync session recreate --force-recreate-session`: explicitly recreate a drifted session.

## Release Builds

```bash
make test
make build
make checksums
make release-snapshot
```

Release metadata is embedded with linker flags. See `Makefile` and `.goreleaser.yaml`.
