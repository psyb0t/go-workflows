# go-workflows

[![Go Reference](https://pkg.go.dev/badge/github.com/psyb0t/go-workflows.svg)](https://pkg.go.dev/github.com/psyb0t/go-workflows)
[![Pipeline](https://github.com/psyb0t/go-workflows/actions/workflows/pipeline.yml/badge.svg?branch=main)](https://github.com/psyb0t/go-workflows/actions/workflows/pipeline.yml)
[![Coverage](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/coverage.svg)](https://github.com/psyb0t/go-workflows/actions/workflows/pipeline.yml)
[![Version](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/version.svg)](https://github.com/psyb0t/go-workflows/tags)
[![License](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/license.svg)](LICENSE)
[![Imported by](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/importers.svg)](https://github.com/psyb0t/go-workflows/blob/badges/importers.md)

Durable workflows in Go. Run activities, wait for signals and timers, and resume from persisted history after a worker restarts.

The engine runs inside your application. Pick SQLite, PostgreSQL, MySQL or Redis for persistence. A monoprocess backend is also available for local work.

## Try it

The host needs Docker, Make and Git. Go and Node tooling run in containers.

```bash
git clone https://github.com/psyb0t/go-workflows.git
cd go-workflows
make run
```

The example starts a worker, registers a workflow and activity, persists their history in `quickstart.sqlite`, waits for the result `42`, and shuts down cleanly. See [the runnable source](samples/quickstart/main.go) and [its test](samples/quickstart/main_test.go).

## Use it in an application

The module is `github.com/psyb0t/go-workflows`. It requires the Go version in [go.mod](go.mod). Select a published tag or commit when adding it to your application's dependencies.

Workflows describe the sequence. Activities perform I/O and other side effects.

```go
func calculate(ctx workflow.Context, left, right int) (int, error) {
    result, err := workflow.ExecuteActivity[int](
        ctx, workflow.DefaultActivityOptions, add, left, right,
    ).Get(ctx)
    if err != nil {
        return 0, ctxerrors.Wrap(err, "add numbers")
    }
    return result, nil
}

func add(ctx context.Context, left, right int) (int, error) {
    return left + right, nil
}
```

Register both functions with `worker.New(backend, nil)`. Start the worker with a cancellable context, create an instance with `client.New(backend)`, then read its typed result with `client.GetWorkflowResult[int]`. The [quick start](samples/quickstart/main.go) includes imports, persistence and worker cleanup.

## Execution rules

Workflow code replays. Use the `workflow` package for time, timers, concurrency, channels and logging. Put network calls, database changes, random values and filesystem operations in activities or recorded side effects. Do not use native goroutines, native channels, wall-clock time or unordered map iteration to make workflow decisions.

Activities have at-least-once execution semantics. A worker can finish an external action and fail before recording its result. Make activities idempotent or deduplicate using a durable operation key. This library does not promise exactly-once external side effects.

Keep inputs and results serializable. Retain compatible workflow definitions while their instances are active. See [the workflow guide](docs/guide.md) for retries, cancellation and upgrades.

## What is included

- Typed activity and subworkflow results, signals, durable timers and retries.
- SQLite, PostgreSQL, MySQL, Redis and monoprocess backends.
- Separate workflow and activity workers, task queues, tracing and metrics hooks.
- Replay-aware logging with `ctxscope` metadata propagation.
- Workflow and activity test helpers.
- An embedded diagnostics UI and a separate [workflow analyzer](analyzer/README.md).

## Documentation

| Read | For |
| --- | --- |
| [Workflow guide](docs/guide.md) | Determinism, retries, signals, cancellation and logging |
| [Backends and operations](docs/backends.md) | Storage, migrations, retention and diagnostics |
| [Development](docs/development.md) | Make commands, tests, tools and dependencies |
| [Releases](docs/releases.md) | Module tags and the default-branch release gate |
| [Analyzer](analyzer/README.md) | Workflow checks and custom lint configuration |
| [Samples](samples/) | Executable feature examples |

## Development

```bash
make help
make format lint build
make test-coverage
make sec
make generate
```

`make test` runs the complete race-enabled suite, including isolated real PostgreSQL, MySQL and Redis services. `make test-unit` excludes external services. Missing Docker is an error in the full suite, not a skipped integration test.

## License and origin

[MIT](LICENSE). Forked from [Christopher Schleiden's go-workflows](https://github.com/cschleiden/go-workflows). The upstream copyright and license remain. The psyb0t module has its own import path and release line.

External pull requests are not accepted. Open an issue with a reproducible example.
