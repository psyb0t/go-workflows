# go-workflows

[![Go Reference](https://pkg.go.dev/badge/github.com/psyb0t/go-workflows.svg)](https://pkg.go.dev/github.com/psyb0t/go-workflows)
[![Pipeline](https://github.com/psyb0t/go-workflows/actions/workflows/pipeline.yml/badge.svg?branch=main)](https://github.com/psyb0t/go-workflows/actions/workflows/pipeline.yml)
[![Coverage](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/coverage.svg)](https://github.com/psyb0t/go-workflows/actions/workflows/pipeline.yml)
[![Version](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/version.svg)](https://github.com/psyb0t/go-workflows/tags)
[![License](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/license.svg)](LICENSE)
[![Imported by](https://raw.githubusercontent.com/psyb0t/go-workflows/badges/importers.svg)](https://github.com/psyb0t/go-workflows/blob/badges/importers.md)

Durable workflows in Go. Run activities, wait for signals and timers, and resume from persisted history after a worker restarts.

The engine runs inside your application. Pick SQLite, PostgreSQL, MySQL or Redis for persistence. A monoprocess backend is also available for local work.

## Contents

- [Try it](#try-it)
- [Use it in an application](#use-it-in-an-application)
- [Execution rules](#execution-rules)
- [What is included](#what-is-included)
- [Documentation](#documentation)
- [Development](#development)
- [License and origin](#license-and-origin)

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

Clients enqueue work in a backend. Workers poll that backend, run registered workflow definitions and activities, and persist their results. A backend is storage and queueing, not a separate workflow server you must deploy. Clients and workers can share one process or run in separate replicas connected to the same storage and queues.

### 1. A client and worker in one process

This complete program registers a greeting workflow, starts background workers, submits an instance, reads `Hello, reader`, and shuts down. No database server is needed. Run it with `make run-greeting`, or copy it into your own module after adding this library.

```go
// This example shows the smallest client/worker connection without activities.
// Run make run-greeting. It prints a greeting, drains the worker and exits.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxscope"

	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := context.Background()
	result, err := run(ctx)
	if err != nil {
		slog.Error("greeting failed", "err", err)
		os.Exit(1)
	}
	ctxscope.GetLogger(ctx).Info("workflow completed", "result", result)
}

func run(ctx context.Context) (result string, err error) {
	// One backend holds the queue and history for both client and worker.
	// In-memory SQLite needs no server, but loses its history when closed.
	b := sqlite.NewInMemoryBackend()
	defer func() {
		if closeErr := b.Close(); closeErr != nil {
			err = errors.Join(err, ctxerrors.Wrap(closeErr, "close backend"))
		}
	}()
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	w := worker.New(b, nil)
	// Only workers register implementations. Clients submit their names and inputs.
	if err := w.RegisterWorkflow(greet); err != nil {
		return "", ctxerrors.Wrap(err, "register greeting")
	}
	// Start already launches background goroutines, so the client can run below.
	if err := w.Start(workerContext); err != nil {
		return "", ctxerrors.Wrap(err, "start worker")
	}
	defer func() {
		// Drain the worker before closing its backend, including on client errors.
		cancel()
		if stopErr := w.WaitForCompletion(); stopErr != nil {
			err = errors.Join(err, ctxerrors.Wrap(stopErr, "stop worker"))
		}
	}()
	c := client.New(b)
	// This enqueues a workflow. The worker picks it up through b, not through RPC.
	instance, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, greet, "reader")
	if err != nil {
		return "", ctxerrors.Wrap(err, "enqueue greeting")
	}
	// Read the persisted result. Timing out here does not cancel the workflow.
	result, err = client.GetWorkflowResult[string](ctx, c, instance, 10*time.Second)
	if err != nil {
		return "", ctxerrors.Wrap(err, "wait for greeting")
	}
	return result, nil
}

func greet(_ workflow.Context, name string) (string, error) {
	// Pure deterministic work can stay in the workflow. I/O belongs in activities.
	return "Hello, " + name, nil
}
```

### 2. A workflow that delegates work to an activity

This complete program adds an activity and file-backed persistence. Run it with `make run`. It returns `42`; queued tasks and history remain in `quickstart.sqlite` after exit. Put external calls in activities, and use workflow APIs to coordinate them.

```go
// This example runs a client and worker in one process against a SQLite file.
// Run it with make run. Each invocation creates a new workflow and prints 42.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxscope"

	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := ctxscope.Set(context.Background(), ctxscope.Attr("request_id", uuid.NewString()))
	result, err := run(ctx, "quickstart.sqlite")
	if err != nil {
		slog.Error("workflow failed", "err", err)
		os.Exit(1)
	}
	ctxscope.GetLogger(ctx).Info("workflow completed", "result", result)
}

func run(ctx context.Context, path string) (result int, err error) {
	// The backend stores queued tasks and history. It is the connection between
	// the client and worker, and the file survives this process exiting.
	b := sqlite.NewSqliteBackend(path)
	defer func() {
		if closeErr := b.Close(); closeErr != nil {
			err = errors.Join(err, ctxerrors.Wrap(closeErr, "close backend"))
		}
	}()
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	w := worker.New(b, nil)
	// Registration maps persisted function names to implementations on this worker.
	// Register before polling so every task has an implementation available.
	if err := w.RegisterWorkflow(calculate); err != nil {
		return 0, ctxerrors.Wrap(err, "register workflow")
	}
	if err := w.RegisterActivity(add); err != nil {
		return 0, ctxerrors.Wrap(err, "register activity")
	}
	// Start launches background pollers and returns. No extra go w.Start is needed.
	if err := w.Start(workerContext); err != nil {
		return 0, ctxerrors.Wrap(err, "start worker")
	}
	defer func() {
		// Stop polling and drain worker goroutines before the earlier defer closes storage.
		cancel()
		if stopErr := w.WaitForCompletion(); stopErr != nil {
			err = errors.Join(err, ctxerrors.Wrap(stopErr, "stop worker"))
		}
	}()
	// The client writes a request to storage; it does not call calculate directly.
	// Other processes can submit work through the same shared backend and queues.
	c := client.New(b)
	instance, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, calculate, 20, 22)
	if err != nil {
		return 0, ctxerrors.Wrap(err, "start workflow")
	}
	// The timeout bounds this client's wait, not the lifetime of the workflow.
	result, err = client.GetWorkflowResult[int](ctx, c, instance, 10*time.Second)
	if err != nil {
		return 0, ctxerrors.Wrap(err, "wait for result")
	}
	return result, nil
}

func calculate(ctx workflow.Context, left, right int) (int, error) {
	// Workflow code replays from history. Schedule an activity and await its
	// recorded result rather than performing external side effects here.
	result, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, add, left, right).Get(ctx)
	if err != nil {
		return 0, ctxerrors.Wrap(err, "add numbers")
	}
	workflow.Logger(ctx).Info("calculation completed", "result", result)
	return result, nil
}

func add(_ context.Context, left, right int) (int, error) {
	// Activities run ordinary Go code. Put network or database work here and make
	// side effects idempotent: a crash before recording success can cause a retry.
	return left + right, nil
}
```

`Start` already launches the worker's goroutines. You do not need `go worker()` around it. To split an application into a producer and worker service, keep registration and `Start` in the worker process and `CreateWorkflowInstance` in the producer. Both must use the same backend and queues. In-memory SQLite only connects objects in one process. See [split workers](samples/simple-split-worker/) and [architecture](docs/architecture.md) for shared storage, replicas and task ownership.

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
- An embedded diagnostics UI.

## Documentation

| Read | For |
| --- | --- |
| [Architecture and workers](docs/architecture.md) | Clients, backends, replicas, identities, queues and shutdown |
| [Workflow guide](docs/guide.md) | Determinism, retries, signals, cancellation and logging |
| [Backends and operations](docs/backends.md) | Storage, connection ownership, migrations and retention |
| [Diagnostics](docs/diagnostics.md) | UI mounting, HTTP API, access control, metrics and tracing |
| [Development](docs/development.md) | Make commands, tools and dependencies |
| [Generated mocks](docs/generation.md) | Mockery sources, output, imports and CI drift checks |
| [Releases](docs/releases.md) | Module tags and the default-branch release gate |
| [Testing](docs/testing.md) | Independent workers, crash recovery and backend contracts |
| [Samples](samples/README.md) | Feature map, executable examples and demo storage cautions |

## Development

```bash
make help
make format lint build
make test-coverage
make sec
make generate
```

`make test` runs the complete race-enabled suite, including isolated real PostgreSQL, MySQL and Redis services. `make test-unit` excludes external services. Missing Docker is an error in the full suite, not a skipped integration test.

See [testing](docs/testing.md) for independent-process recovery scenarios, coverage scope and focused commands.

## License and origin

[MIT](LICENSE). Forked from [Christopher Schleiden's go-workflows](https://github.com/cschleiden/go-workflows). The upstream copyright and license remain. The psyb0t module has its own import path and release line.

External pull requests are not accepted. Open an issue with a reproducible example.
