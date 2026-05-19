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

## Drift

Session drift means the existing Mutagen session no longer matches the resolved local endpoint, remote endpoint, or ignore rules. DevSync reports drift and requires explicit recreation.
