# Examples

## Global Config

```yaml
nodes:
  core-dev:
    ssh: core-dev
    workspace_root: ~/workspace
defaults:
  ignores:
    - .git
    - node_modules
    - dist
    - build
mapping:
  local_root: ~/remote
```

## Workspace Override

```yaml
workspace: steel-api
remote:
  ssh:
    user: dev
    host: 100.72.16.64
    port: "22"
  path: /home/dev/workspace/work/custom-location
sync:
  ignores:
    - .env.local
```

## First Sync

Recommended golden path:

1. Start from a clean local clone.
2. Seed the remote from the latest local mirror or clone.
3. Do not edit either working tree after seeding.
4. Run dry-run immediately.
5. Run first sync immediately after reviewing the plan.

```bash
devsync bootstrap --init-workspace
devsync init-remote
devsync doctor
devsync sync --dry-run
devsync sync
```

If local changes happen after remote seeding but before the first Mutagen session, DevSync may report initial sync risk or Mutagen may surface conflicts. Resolve manually; do not expect automatic convergence.

## Divergence Recovery

```bash
devsync status
git fetch <remote>
git log --oneline --graph --left-right HEAD...<remote-branch>
```

Resolve manually, then rerun `devsync sync --dry-run`.
