# Development

Use the repository's Docker-backed Make targets for Go, UI, dependency and generation work. Read `docs/development.md` and `docs/testing.md` before changing build or verification commands. Do not restore the removed analyzer module or a shared test Compose stack.

Run normal tests with the race detector. `make test-coverage` includes real backend and independent-process recovery tests and enforces a 90% floor. Missing Docker or a failed backend is a failure, not a reason to skip or weaken the suite.

Generated output is declared by a package-local `gen.go` and regenerated with `make generate`. Never edit generated files or vendor sources. Keep examples executable and synchronized with the README programs.

Workflow code must replay deterministically. Put I/O in activities, use workflow timer/channel/concurrency APIs, and keep activity effects idempotent. Queues and worker names do not provide authorization.
