# stepthrough — Claude guidance

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
