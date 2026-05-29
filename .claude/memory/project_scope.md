---
name: project-scope
description: Scope constraints for the stepthrough Azure pipeline debugger project
metadata:
  type: project
---

Only ubuntu-latest Docker images are supported for local pipeline debugging (running on macOS via Docker Desktop). Do not add support for windows-latest, macos-latest, or other pool vmImage values — reject them with a clear error message.

**Why:** User explicitly scoped this down. The project runs on macOS and only Linux containers work in Docker Desktop.

**How to apply:** Keep `runner.ResolveImage` simple — ubuntu-latest/ubuntu-22.04 → ubuntu:22.04, unknown custom strings pass through, everything else returns unsupported. Do not expand the image matrix.
