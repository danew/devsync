# Synchronization Semantics

## Git Is Authoritative

DevSync compares local and remote commit ancestry before filesystem synchronization.

```text
0 0 -> equal, safe to sync
3 0 -> local ahead, safe to push then sync
0 2 -> remote ahead, safe to fast-forward pull then sync
2 4 -> diverged, abort
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

## Drift

Session drift means the existing Mutagen session no longer matches the resolved local endpoint, remote endpoint, or ignore rules. DevSync reports drift and requires explicit recreation.
