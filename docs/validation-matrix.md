# Release Validation Matrix

Use this matrix before public releases.

| Scenario | Expected Behavior | Recovery |
| --- | --- | --- |
| macOS local + Linux remote | `doctor`, `status`, `sync --dry-run`, `sync` pass | Use `doctor` timings to diagnose SSH/Mutagen issues |
| Nested workspace path | Recursive mapping preserves subpath | Override `.devsync.yaml` if convention is wrong |
| Monorepo | Dirty tree visible, Git ancestry authoritative | Use ignores for generated directories |
| Large repository | Mutagen handles file scan; DevSync remains CLI-only | Inspect `mutagen sync list` for scan/transport problems |
| Dirty working tree | Warns in status, continues only if ancestry safe | Commit/stash manually if desired |
| Interrupted sync | Lock prevents concurrent orchestration; next run revalidates | Remove stale lock only after process check |
| Paused Mutagen session | `sync` resumes explicitly and flushes | `devsync session inspect` |
| Remote reboot | SSH/Mutagen errors fail closed | Wait for remote, run `doctor` |
| Stale lockfile | Old lock is recovered automatically after stale threshold | Manual removal only if safe |
| Network interruption | SSH/Mutagen command fails; no hidden repair | Rerun `doctor`, then `sync --dry-run` |
| Branch divergence | Abort before filesystem sync | Resolve manually with Git |
| Path drift | Session drift reported, no silent recreation | `session recreate --force-recreate-session` after inspection |
| Remote repo deletion | Remote validation fails clearly | Reclone remote repo or update config |
| SSH failure | Remote unreachable error with remediation | Fix SSH config/connectivity |

Release readiness requires validating at least one real macOS to Linux workspace plus all automated tests.
