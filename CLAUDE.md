# stepthrough — Claude guidance

> **Before making any structural changes, read [`ARCHITECTURE.md`](ARCHITECTURE.md).** It tracks the module map, known architectural debt, and decisions not to relitigate.

## Project overview

Desktop app (Wails v3 + React/TypeScript) for running Azure Pipelines locally with hot-refresh. Go backend manages Docker containers and pipeline execution; React frontend renders pipeline state via Wails events.

## Key architecture

- `orchestrator/` — pipeline execution engine (one `Orchestrator` per pipeline file)
- `service/` — Wails-registered adapters (`PipelineService`, `WatcherService`, `UIService`)
- `session/` — disk persistence (`~/.config/stepthrough/session.json`)
- `pipeline/` — YAML parser and data model
- `runner/` — Docker executor
- `frontend/src/` — React app; state managed via `useReducer` in `App.tsx`

All pipeline events are wrapped as `PipelineFileEvent{File, Data}` so the frontend can route them to the correct tab.

## Agent image (`agent/Dockerfile`)

The `agent/` Dockerfile builds `ghcr.io/kascea/stepthrough-agent:latest` — the container stepthrough uses to run pipeline steps locally. **It must mirror Azure's `ubuntu-latest` hosted agent as closely as possible**, because the whole point of the app is that pipelines run the same way locally as they do in Azure.

This means:
- Same OS base (`ubuntu:24.04`, matching Azure's current `ubuntu-latest`)
- Same pre-installed toolchain versions (Go, Node, .NET, Python)
- Same system libraries as Azure's hosted agent — do not add packages that Azure doesn't ship (e.g. GTK4/WebKit dev headers are absent from Azure's agent and must stay absent here too)
- When Azure bumps a tool version, bump the agent image to match

Do not add packages to the Dockerfile without first confirming they are present on Azure's `ubuntu-latest` hosted agent. The goal is parity, not a fully-featured dev image.

The CI pipeline itself (`examples/azure-pipelines.yml`) runs on Microsoft-hosted agents (not the stepthrough-agent image). Because those agents lack GTK4 headers, Go packages that need CGo/Wails must be gated behind `//go:build !server` so that `go build/test -tags=server ./...` in CI skips the Wails desktop layer entirely.

## Running the app

```
wails3 dev          # hot-reload dev mode
wails3 build        # production build
go test ./...       # Go tests
cd frontend && npm test   # frontend tests (Vitest)
```

## Testing rules

**After fixing a bug, always write a test that would have caught it.**

- Go bugs → add a test in the same package (e.g. `service/pipeline_test.go`)
- Frontend reducer/logic bugs → add a unit test in `frontend/src/__tests__/`
- The test must fail on the unfixed code and pass on the fix

This rule exists because the `GetSession` nil-tabOrder bug (Go nil slice → JSON `null` → `null.length` crash in the reducer) went undetected and caused a white screen on first launch. The regression tests in `service/pipeline_test.go` and `frontend/src/__tests__/reducer.test.ts` now guard against it.

## Go ↔ Frontend data contract

- Go `nil` slices marshal to JSON `null` — always initialise slices before returning from service methods (use `[]T{}` not nil)
- Frontend reducers must null-guard any data from RPC calls (use `?.length`, `?? []`, etc.)
- Service method names become Wails RPC identifiers: `Call.ByName('...service.ServiceName.MethodName')`

## Session restore sequence

1. Frontend mounts → calls `GetSession()` → initialises tabs from saved data
2. Go goroutine (400 ms delay) → `RestoreTab(file)` per saved file → emits `pipeline:loaded` or `pipeline:missing`
3. Watcher registered for files that exist → subsequent saves trigger hot-reload
