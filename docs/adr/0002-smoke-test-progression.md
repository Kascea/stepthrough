# ADR 0002 — Example Pipeline Smoke Test Progression

**Status:** Active  
**Date:** 2026-06-07

## Context

`examples/` contains real-world `azure-pipelines.yml` files from public repos, used to verify stepthrough's parser and runner against realistic input. These pipelines are exercised at two levels, mirroring the existing unit/integration split:

- **Parse tests** (always run, no Docker) — assert the pipeline model is built correctly.
- **Runner smoke tests** (integration tag, requires Docker) — execute the pipeline end-to-end.

The question is: what should a passing runner smoke test assert?

## Decision

We adopt a **two-phase progression**:

**Phase 1 (current):** A smoke test passes if the pipeline reaches `pipeline:done` without a `pipeline:setup-error` or a Go panic. This is the "it doesn't blow up" floor. It catches adapter panics, YAML structures the runner can't handle, and Docker executor failures.

**Phase 2 (promote when stable):** Once the example suite runs cleanly, promote key pipelines to assert all steps exit 0. This catches silent failures — steps that run but produce wrong exit codes.

We explicitly defer step-status fixture diffing (asserting exact `StepStatus` values per step) until the live Azure comparison system (ADR 0001) is in place, since that's the natural home for "did we get the same result as Azure?"

## Why not jump straight to exit-code assertions

Phase 1 gives immediate signal on the broadest class of failures without requiring that every step in a real-world pipeline succeeds in the test environment (network access, credentials, etc. may not be available). Stabilise the floor first.

## Consequences

- Phase 1 tests are cheap to write and maintain; they only require Docker to be running.
- Phase 2 assertions may need environment-specific skips (e.g. steps that fetch from the internet).
- The example corpus and the live comparison system (ADR 0001) share the same `examples/` directory — the progression from Phase 1 → 2 → comparison is additive, not a rewrite.
