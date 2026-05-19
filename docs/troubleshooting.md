# Troubleshooting

## Start Here

```bash
devsync doctor
devsync status
devsync sync --dry-run
```

## Branch Mismatch

DevSync aborts when local and remote branches differ.

Recovery:

```bash
git branch --show-current
ssh <host> 'cd <repo> && git branch --show-current'
```

Manually checkout matching branches. DevSync will not switch branches for you.

## Divergence

DevSync aborts when both sides are ahead.

Recovery:

```bash
git log --oneline --graph --left-right local...remote
```

Resolve with normal Git workflows, then rerun `devsync status`.

## Session Drift

Inspect first:

```bash
devsync session inspect
mutagen sync list
```

If the drift is expected:

```bash
devsync session recreate --force-recreate-session
```

## Initial Sync Risk

If DevSync reports `initial synchronization risk detected`, no Mutagen baseline exists yet and one or both working trees are dirty or not at matching HEADs.

Recommended recovery:

1. Clean or intentionally preserve local changes.
2. Recreate the remote clone from the latest local repository state, or manually make the remote working tree match.
3. Run `devsync status`.
4. Run `devsync sync --dry-run`.
5. Run `devsync sync`.

In scripts or CI, DevSync will not prompt. It fails closed instead.

## Mutagen Conflicts

Mutagen conflicts mean filesystem contents differ in ways Mutagen will not resolve automatically.

Inspect:

```bash
devsync status
mutagen sync list --long devsync-<workspace>
```

Safe recovery:

```bash
mutagen sync terminate devsync-<workspace>
```

Then manually reconcile local and remote working tree differences, verify Git state, and recreate the session explicitly:

```bash
devsync sync --dry-run
devsync sync
```

DevSync will not choose winners or overwrite files for you.

## Stale Lock

`devsync doctor` prints lockfile metadata. Remove a lock only after confirming no `devsync` process is running.

## Missing Remote Repo

Clone or create the repository remotely, or update `.devsync.yaml` with the correct `remote.path`.
