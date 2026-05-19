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
  node: core-dev
  path: ~/workspace/work/custom-location
sync:
  ignores:
    - .env.local
```

## First Sync

```bash
devsync bootstrap --init-workspace
devsync doctor
devsync sync --dry-run
devsync sync
```

## Divergence Recovery

```bash
devsync status
git fetch <remote>
git log --oneline --graph --left-right HEAD...<remote-branch>
```

Resolve manually, then rerun `devsync sync --dry-run`.
