DevSync — Project Overview

Vision

DevSync is a Git-aware workspace synchronization and orchestration tool designed for remote-first software development.

The core idea is simple:

* remote machines become primary development environments
* local machines become optional synchronized workspaces
* synchronization becomes intentional, safe, and Git-aware

DevSync is not intended to replace:

* Git
* SSH
* Mutagen
* tmux
* VSCode Remote SSH

Instead, it composes these existing primitives into a cohesive developer workflow.

⸻

Problem Statement

Modern remote development workflows are fragmented.

Existing tools solve only pieces of the problem.

Tool	Limitation
rsync	Unsafe bidirectional workflows
scp	Stateless and manual
Syncthing	Multi-master complexity
Mutagen	No Git awareness
VSCode Remote SSH	Editor-only workflow

The missing layer is:

Git-aware workspace synchronization and orchestration.

⸻

Initial Motivation

The initial workflow motivating DevSync:

* primary development occurs on a remote Ubuntu server
* access occurs through SSH, browser IDEs, and VSCode Remote SSH
* local development is occasional but still important
* synchronization should be safe and ergonomic
* branch divergence should never silently corrupt work

The workflow should feel like:

devsync

from anywhere inside a repository.

⸻

Core Philosophy

Remote-First Development

The remote machine is treated as the canonical development environment.

Local workspaces are:

* optional
* temporary
* convenience-oriented
* session-based

This dramatically simplifies synchronization semantics.

⸻

Workspace-Oriented Design

DevSync operates on:

* workspaces
* not arbitrary folders

A workspace represents:

* source code
* Git state
* working tree
* environment
* synchronization state
* associated runtime context

The important abstraction is:

workspace state

not raw filesystem replication.

⸻

Git Is Authoritative

Git determines synchronization safety.

Filesystem synchronization only occurs after Git state validation succeeds.

DevSync intentionally separates:

* Git history management
* working tree synchronization

This is one of the most important architectural decisions.

⸻

.git Is Never Synchronized

The .git directory is intentionally excluded from synchronization.

Reasons:

* avoid corruption
* avoid lock contention
* avoid concurrent ref updates
* avoid sync races
* preserve repository integrity

Instead:

* local and remote repositories remain independent
* DevSync reconciles them intentionally using Git semantics

⸻

Synchronization Model

DevSync uses:

* Mutagen for working tree synchronization
* Git for ancestry validation
* SSH for transport

Mutagen handles:

* filesystem watching
* incremental synchronization
* transport resilience
* reconnection
* performance

DevSync handles:

* workspace semantics
* Git validation
* synchronization safety
* orchestration

⸻

Safety Philosophy

DevSync should always prefer:

* explicitness
* predictability
* transparency
* aborting safely

rather than:

* guessing intent
* automatic repair
* hidden state mutation
* automatic merge resolution

⸻

Divergence Policy

If local and remote histories diverge:

local ahead > 0
remote ahead > 0

DevSync aborts.

The user resolves divergence manually.

This is a foundational trust decision.

⸻

Branch Policy

Synchronization only occurs when:

* branch names match
* ancestry is safe

If:

local: feature/foo
remote: main

DevSync aborts.

Initial versions intentionally avoid:

* automatic branch switching
* automatic branch creation
* automatic rebasing
* automatic merge handling

⸻

Workspace Layout

Remote

~/workspace/{personal,work,experiments,archives}

Local

~/remote/{personal,work,experiments,archives}

This provides:

* symmetry
* clean organization
* future scalability
* multi-node compatibility

⸻

Initial Migration Strategy

The initial migration intentionally avoids live synchronization.

Repositories are first mirrored manually.

Example:

git clone --mirror myrepo myrepo.git
scp -r myrepo.git user@server:~/tmp/

Then cloned remotely:

git clone ~/tmp/myrepo.git ~/workspace/work/myrepo

This safely preserves:

* branches
* tags
* refs
* history

before introducing synchronization.

⸻

Long-Term Direction

Although v1 remains intentionally narrow, the broader architecture points toward workspace orchestration.

Potential future capabilities:

* workspace migration
* multi-node development
* service forwarding
* node lifecycle management
* remote wake/suspend
* runtime orchestration
* portable development environments

⸻

Future Workspace Model

Eventually a workspace may include:

* repositories
* services
* forwarded ports
* tmux sessions
* containers
* environment metadata
* node placement

Example future concepts:

devsync migrate --to core-dev

or:

devsync resume workspace-name

These are intentionally deferred until after the synchronization model is proven stable.

⸻

Multi-Node Vision

Future environments may include:

* personal servers
* proxmox clusters
* work servers
* intermittently available machines
* mobile development machines

However:

v1 intentionally assumes:

* one local machine
* one remote machine
* one synchronization relationship

Keeping the initial implementation constrained is critical.

⸻

Networking Philosophy

DevSync intentionally avoids replacing networking infrastructure.

Expected existing primitives:

* SSH
* Tailscale
* existing remote access infrastructure

DevSync orchestrates workflows rather than owning connectivity.

⸻

Daemon Philosophy

Initial versions should avoid long-running daemons.

Commands should:

1. inspect state
2. perform orchestration
3. exit cleanly

This keeps the system:

* debuggable
* inspectable
* trustworthy
* composable

⸻

Tooling Philosophy

If DevSync disappeared entirely:

* repositories remain valid Git repositories
* Mutagen sessions remain usable
* SSH remains usable
* tmux remains usable

DevSync should never own underlying state.

It orchestrates workflows using existing proven primitives.

⸻

Suggested Technology Stack

Language

Go.

Reasons:

* static binaries
* strong concurrency support
* good SSH ecosystem
* cross-platform compatibility
* operational simplicity
* alignment with Mutagen ecosystem

⸻

Initial Scope Boundary

Initial versions intentionally exclude:

* automatic merge resolution
* distributed synchronization
* multi-master replication
* container orchestration
* service orchestration
* cluster scheduling
* node management
* daemon architectures
* peer-to-peer discovery
* authority election

The goal is:

a safe, trustworthy synchronization layer for remote-first development.

⸻

Guiding Principle

DevSync should make remote-first development:

* safer
* simpler
* more ergonomic
* more predictable

without hiding operational complexity from the user.