---
name: stepthrough
description: Glossary of domain terms for the stepthrough project
---

## Fixture

A synthetic, minimal `azure-pipelines.yml` file checked into `examples/fixtures/`, used exclusively by parser unit tests. Fixtures are hand-authored to isolate specific YAML constructs (e.g. `depends-on`, `continue-on-error`). They are not meant to represent real pipelines and are not run through the Docker runner.

**Contrast with:** Example Pipeline

---

## Example Pipeline

A real-world `azure-pipelines.yml` file collected from a public open-source repository, checked into `examples/<repo-name>/`. Example pipelines are used to test the full parser + runner stack against realistic input. They are exercised at two levels: parse tests (always) and runner smoke tests (integration tag, requires Docker).

**Contrast with:** Fixture  
**See also:** ADR 0002 (smoke test progression)

---

## Smoke Test

A runner-level integration test that executes an Example Pipeline through the Docker runner and asserts the pipeline reaches `pipeline:done` without a setup error or panic. The floor assertion — "it doesn't blow up." Distinct from a parse test (no Docker) and from a parity test (no Azure comparison).

**See also:** ADR 0002

---

## Parity Test

A test that runs the same pipeline through stepthrough locally *and* on a real Azure DevOps organisation, then diffs the results. The gold-standard check that stepthrough matches Azure 1:1. Not yet built.

**See also:** ADR 0001

---

## Agent Image

The Docker image (`ghcr.io/kascea/stepthrough-agent`) used as the execution environment for pipeline steps. It mirrors the Azure `ubuntu-latest` hosted agent: same OS base, same pre-installed toolchain versions. Pipeline scripts that need root must use `sudo`, matching the `vsts` non-root user on Azure.

---

## Task Adapter

A local reimplementation of an Azure Pipelines task (e.g. `UseDotNet@2`, `DotNetCoreCLI@2`) that translates the task's inputs into a shell script executable inside the Agent Image. Adapters are registered in `runner/task_metadata.go`. Tasks with no registered adapter fail the pipeline step with `exit 1`.

**See also:** ADR 0003

---

## CI Pipeline

stepthrough's own Azure DevOps pipeline, located at `.azure/pipelines/ci.yml`. Runs lint, build, unit tests, integration tests, and agent image publishing. Not an example pipeline — it is the project's own CI, not representative user content.

**Contrast with:** Example Pipeline
