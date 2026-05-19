# Synchronization Semantics

## Git Is Authoritative

DevSync compares local and remote commit ancestry before filesystem synchronization.

```text
0 0 -> equal, safe to sync
3 0 -> local ahead, abort; synchronize Git history manually
0 2 -> remote ahead, abort; synchronize Git history manually
2 4 -> diverged, abort
```

DevSync does not pull from or push to the peer workspace clone. The remote workspace repository is another working clone, not canonical Git authority. Use normal Git commands against canonical remotes such as `origin` to make local and remote workspace HEADs equal before running `devsync sync`.

Example remediation for a typical remote-ahead local repository:

```bash
git pull --ff-only origin main
```

## `.git` Is Never Synchronized

Mutagen sessions always include `.git` in ignore rules. Users cannot disable this. Local and remote repositories remain independent and valid if DevSync disappears.

## Dirty Working Trees

Dirty working trees are visible in status. DevSync allows them only after ancestry and branch checks pass.

## Initial Synchronization Baseline

Git history safety is not the same as working tree parity.

On the first sync, before a Mutagen session exists, DevSync treats dirty local or remote working trees as an initial synchronization risk. In interactive terminals it asks for explicit confirmation before creating the first session. In non-interactive environments it fails closed with remediation guidance.

Recommended first-sync hygiene:

1. Start from a clean local clone.
2. Seed the remote from the latest local mirror or clone.
3. Avoid local or remote working tree edits after seeding.
4. Run `devsync sync --dry-run`.
5. Run `devsync sync` immediately after the dry-run looks correct.

DevSync does not automatically converge divergent working trees. Mutagen conflicts are surfaced for manual recovery.

## One-Shot Synchronization

`devsync sync` is intentionally bounded. It resumes or creates the Mutagen session, flushes synchronization, then pauses the session before exiting.

This means filesystem propagation happens during explicit DevSync operations, not continuously in the background. If continuous synchronization is desired, use `devsync attach` intentionally and `devsync detach` when finished.

Status output shows the active mode:

```text
mode: one-shot
```

or:

```text
mode: attached (continuous)
```

## Drift

Session drift means the existing Mutagen session no longer matches the resolved local endpoint, remote endpoint, or ignore rules. DevSync reports drift and requires explicit recreation.
