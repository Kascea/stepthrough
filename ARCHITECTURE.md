# stepthrough — Architecture

This file tracks structural observations and deepening opportunities. Update it as decisions are made. It is not a spec — it records what is true and what is worth changing.

---

## Module map

| Package | Depth | Notes |
|---------|-------|-------|
| `pipeline/` | Deep | Clean parse → model → flatten chain. Well-tested. |
| `session/` | Shallow | `Load`/`Save` only. Silently swallows errors; callers must guard nil slices. |
| `runner/` | Moderate | Good internal `stepHandler`/`taskAdapter` interfaces; task metadata caching is fragile. |
| `orchestrator/` | Moderate | `Executor` interface is clean. Goroutine lifecycle is opaque to callers (see improvement #3). |
| `service/` | Shallow (God) | `PipelineService` conflates tab lifecycle, log capture, session persistence, event wrapping, and watcher coordination (see improvement #1). |
| `frontend/App.tsx` | Moderate | Clean `useReducer` pattern. `step:done` event forces an extra RPC round-trip (see improvement #2). |
| `frontend/WatcherService` | Shallow | Bidirectional dependency with `PipelineService`; zero tests (see improvement #4). |

---

## Event contract (Go → Frontend)

All events are wrapped as `PipelineFileEvent{File, Data}` so the frontend can route to the correct tab.

| Event | Payload | Notes |
|-------|---------|-------|
| `pipeline:loaded` | `PipelineState` | Full state snapshot |
| `pipeline:missing` | — | File not found on disk |
| `pipeline:error` | `string` | YAML parse error |
| `pipeline:setup-error` | `string` | Runtime setup failure |
| `pipeline:done` | — | Execution finished |
| `step:started` | `int` (step index) | |
| `step:done` | `int` (step index) | ⚠ frontend immediately calls `GetPipelineState` RPC — see improvement #2 |
| `step:log` | `LogLine` | Streamed log line |
| `setup:log` | `string` | Docker setup log |

---

## Known architectural debt

### #1 · Split `PipelineService` — **Strong** · ✅ done

**Files:** `service/pipeline.go`, `service/tab_manager.go` (new), `service/log_store.go` (new)

Extracted `tabManager` (orchestrators + tab order + activeFile, own mutex) and `logStore` (bounded log capture, own mutex). `pipeline.go` dropped from 361 → ~200 lines and is now a thin façade.

**Key correctness fix included:** `GetSession` now calls `tabs.seedOrder()` (order only, no orchestrators) and `RestoreTab` calls `tabs.addOrch()` (orchestrator only, no order change), matching the original two-phase session restore sequence. `persistSession` now deletes runs based on tab order rather than orchestrator presence, preventing saved run data loss during the 400ms window before `RestoreTab` runs.

---

### #2 · Eliminate `step:done` RPC round-trip — **Strong** · ✅ done

**Files:** `frontend/src/App.tsx` (lines 335–341), `orchestrator/orchestrator.go`, `service/pipeline.go`

When Go emits `step:done` it only sends the step index. The frontend immediately fires `GetPipelineState` RPC to get the full pipeline state, creating:
- A race window between `step:done` and the next `step:started`
- A bidirectional data dependency (frontend knows it needs more than the event provides)

**Fix:** Emit the full `PipelineState` as the payload of `step:done`. The orchestrator already has it; the frontend reducer can apply it atomically with no RPC.

**Implemented:** `orchestrator/orchestrator.go` now emits `o.GetState()` on both `step:done` sites. Frontend handler simplified to `unwrap<PipelineState>` with a direct dispatch — no RPC. `GetPipelineState` confirmed to be a pure passthrough to `orch.GetState()` with no log merging, so the snapshot is complete. Regression tests added in `frontend/src/__tests__/reducer.test.ts`.

---

### #3 · Explicit `Orchestrator` goroutine lifecycle — **Worth exploring** · not started

**Files:** `orchestrator/orchestrator.go` (lines 185–295), `orchestrator/orchestrator_test.go`

`RunFrom()` spawns a goroutine and returns immediately. Callers have no way to know when execution finishes except by awaiting `pipeline:done` via the event sink. This makes double-start safety and test synchronisation fragile.

**Fix:** Add `Done() <-chan struct{}` to `Orchestrator`. Channel closes when the run goroutine exits. Complementary to the event sink — not a replacement.

---

### #4 · Break `WatcherService` ↔ `PipelineService` feedback loop — **Worth exploring** · not started

**Files:** `service/watcher.go`, `service/pipeline.go`

Bidirectional dependency: `PipelineService` calls `WatcherService.AddWatch()`; `WatcherService` calls back into `PipelineService.ReloadFile()`. Neither can be tested or understood independently. `WatcherService` has zero tests.

**Fix:** Give `WatcherService` an `OnChange func(file string)` seam set at construction. `PipelineService` passes its own `ReloadFile` as the callback. `WatcherService` no longer imports `PipelineService`.

---

### #5 · `ScriptContext` for Azure variable expansion — **Speculative** · not started

**Files:** `runner/variables.go`, `runner/handlers.go:41`, `runner/task_metadata.go:53`, `runner/task_adapters.go`

`expandAzureVariables()` is called in five scattered places. Any new adapter that forgets to expand variables silently produces wrong output.

**Fix:** Introduce `ScriptContext{vars}` with an `Expand(string) string` method. Thread it through the step resolution chain so expansion is enforced by the type system.

---

## Tab lifecycle state machine

Defined as `TabStatus` in [service/tab_manager.go](service/tab_manager.go) (Go) and as the `TabStatus` discriminated union in [frontend/src/types.ts](frontend/src/types.ts) (TypeScript). Both sides use the same five states. Transitions:

```
seedOrder / ensure / addOrch
        │
        ▼
     Empty ──── RestoreTab (file missing) ──▶ Missing
        │
        │ RestoreTab / ReloadFile (parse ok)
        ▼
     Loaded ◀──────────────────────────────── pipeline:done
        │                                          ▲
        │ RunPipeline / ReloadFile (starts run)    │
        ▼                                          │
     Running ──────────────────────────────────────┘
        │
        │ pipeline:setup-error
        ▼
     Error ◀─── RestoreTab / ReloadFile (parse error)
```

`Missing` and `Error` are terminal for the current load; the user must relocate or fix the file (which goes through `ReloadFile` → `Loaded`/`Error`).

## Decisions not to relitigate

*(Empty — record rejections here with rationale so future reviews don't re-suggest them.)*
