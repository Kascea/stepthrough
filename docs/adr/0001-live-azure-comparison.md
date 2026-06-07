# ADR 0001 — Live Azure Comparison (deferred)

**Status:** Deferred — not started  
**Date:** 2026-06-07

## Context

stepthrough's core promise is that pipelines run the same way locally as they do on Azure. The current testing strategy (A) uses curated real-world `azure-pipelines.yml` files checked into `examples/`, run through the Docker runner locally, with assertions on step exit codes. This catches regressions in the parser and runner but cannot detect subtle parity drift — cases where stepthrough and Azure both succeed but produce meaningfully different behaviour (different environment variables, different tool versions resolved, different working directory state).

## Decision

We will eventually build a **live Azure comparison system** ("Option B") alongside the existing example suite. This system will:

1. Take a curated pipeline (from `examples/`) and run it through stepthrough locally.
2. Simultaneously trigger the same pipeline on a real Azure DevOps organisation via the Azure Pipelines REST API.
3. Collect structured results from both runs (step statuses, exit codes, selected log output).
4. Diff the two result sets and fail if they diverge beyond a defined tolerance.

This is a separate concern from the example-based smoke tests (see ADR 0002). The example suite proves stepthrough doesn't break; the comparison system proves stepthrough matches Azure.

## Why deferred

- Requires Azure credentials, a live org, and a dedicated pipeline project — infrastructure we don't have yet.
- Parallel run coordination (polling the Azure REST API, aligning step boundaries) is non-trivial.
- The example-based smoke tests give good coverage for now and are the right foundation to build on.

## Consequences

- When we build this, the comparison system consumes `examples/` pipelines — it is not a separate corpus.
- The tolerance definition (what counts as "same enough") will need its own decision when the time comes.
- Azure API authentication will likely require a PAT stored as a CI secret, not committed.
