# ADR 0003 — Local-Only Task Adapters (no remote task definition fetch)

**Status:** Active  
**Date:** 2026-06-07

## Context

Azure Pipelines tasks (e.g. `UseDotNet@2`, `DotNetCoreCLI@2`) are defined in the public [microsoft/azure-pipelines-tasks](https://github.com/microsoft/azure-pipelines-tasks) repository. Each task ships a `task.json` metadata file describing its inputs, and a compiled Node.js bundle (e.g. `usedotnet.js`) containing the actual execution logic.

stepthrough previously downloaded `task.json` from GitHub at runtime to resolve input names, aliases, and defaults, caching the results in `~/Library/Caches/stepthrough/`. The actual task logic was never downloaded — stepthrough has its own local reimplementations (adapters) for each supported task.

## Decision

stepthrough uses **local-only task adapters**. Input definitions (names, aliases, defaults) are hardcoded in `runner/task_metadata.go` alongside each adapter. There is no network fetch, no cache, and no dependency on the GitHub raw content URL.

Tasks absent from `localTasks` fail the pipeline step immediately with `exit 1` and a clear message naming the unsupported task.

## Why not run the real task bundles

The `task.json` is metadata only — the actual task logic lives in compiled `.js` bundles that are not distributed via the raw GitHub URL. Those bundles import `azure-pipelines-task-lib`, a library that communicates with the Azure agent process via a proprietary socket protocol. Running them locally would require stubbing the entire agent API surface, which is effectively reimplementing the Azure agent — a far larger project.

## Why not fetch task.json for input resolution

The remote fetch was providing input defaults and aliases. These are small, stable, and already fully captured in the adapters we write. Hardcoding them alongside the adapter eliminates:

- A runtime network dependency that caused intermittent "task definition not found" failures
- A disk cache that could become stale, corrupt, or permission-denied
- Test infrastructure (mock HTTP server) required to exercise task resolution

The only cost is that when a new task version changes its input schema, we update the local definition manually. Given that we also have to update the adapter logic itself, this is not additional burden.

## Consequences

- Adding support for a new task requires both a local adapter and an entry in `localTasks` — there is no auto-discovery.
- Unsupported tasks fail loudly rather than silently passing, making gaps in coverage visible.
- Task resolution is now instantaneous and works offline.
