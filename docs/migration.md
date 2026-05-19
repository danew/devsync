# Migration Guidance

DevSync v1 does not migrate workspaces automatically.

Recommended setup is still explicit Git cloning:

```bash
ssh core-dev 'mkdir -p ~/workspace/work'
ssh core-dev 'cd ~/workspace/work && git clone <repo-url> example'
mkdir -p ~/remote/work
git clone <repo-url> ~/remote/work/example
cd ~/remote/work/example
devsync bootstrap --init-workspace
devsync doctor
devsync sync --dry-run
```

This keeps local and remote repositories independent and avoids synchronizing `.git`.

Do not copy live working trees into place with bidirectional sync enabled. Establish valid Git repositories first, then let DevSync validate ancestry before synchronizing working-tree changes.
