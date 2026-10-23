# Generated mocks

Mockery is pinned in the root `go.mod` tool block and included in `vendor`. Generators run in the development image through `make generate`, which invokes `go generate ./...`.

## Sources and outputs

| Interface | Configuration and directive | Generated output |
| --- | --- | --- |
| `backend.Backend` | `backend/mocks/config.yml`, `backend/mocks/gen.go` | `backend/mocks/backend.gen.go` |
| `internal/worker.TaskWorker` | `internal/worker/mocks/config.yml`, `internal/worker/mocks/gen.go` | `internal/worker/mocks/task_worker.gen.go` |

Each `gen.go` contains only its package clause and `//go:generate go tool mockery --config config.yml`. Configuration paths and output directories are relative to that package. Adding another generated package requires its own directive and configuration, not another Make target.

Generated filenames use `<interface>.gen.go` in snake_case, with no `mock_` prefix. They belong in a dedicated `mocks` package. Commit generated output together with its interface and configuration changes. Do not edit generated files or vendor sources manually.

## Using the mocks

Import `github.com/psyb0t/go-workflows/backend/mocks` for `MockBackend` or `NewMockBackend(t)`. The constructor registers expectation checks with test cleanup. `MockTaskWorker[Task, Result]` is generic; worker tests instantiate it with their task/result fixtures.

Mocks are for unit boundaries. Backend contracts and process recovery tests still use real implementations and production migrations. See [testing](testing.md).

## Changing an interface

1. Edit the handwritten interface and any affected consumers.
2. Update its Mockery configuration when its package, name or output changes.
3. Run `make generate` and review the source plus generated diff.
4. Run generation again and verify identical contents.
5. Run `make lint build` and the affected race-enabled tests.

`*.gen.go` and directive-only `gen.go` files are excluded from coverage reporting, not from Git. Handwritten source remains measured.

## CI protection

The shared code workflow runs `make generate` after build, lint, tests and security checks. It fails when the working tree has modified, deleted or untracked files. This catches stale output, hand-edits and newly generated files that were never committed. `git diff` alone would miss new untracked output.

A clean checkout must remain clean after generation. Check `git status --porcelain --untracked-files=all` locally. Ignoring a generated file is not a way to satisfy this gate.
