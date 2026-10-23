# Samples

Start with [quickstart](quickstart/). `make run` executes it in the development container, prints result `42` and shuts down its worker and backend. It needs no external database service.

For the smallest client/worker example without activities, use `make run-greeting`. Both programs appear in full in the [root README](../README.md#use-it-in-an-application), and tests keep those copies synchronized with executable source.

## Reading a sample

Each entry point explains its command, backend requirements, expected outcome and whether it exits or waits for Ctrl+C. Feature comments explain the behavior being demonstrated, such as replay-safe concurrency, queue routing or carrying state across executions. Follow the client submission, worker registration and workflow definition in that order. Activities use ordinary `context.Context`; workflow functions use `workflow.Context` and replay-safe APIs.

`worker.Start` launches background pollers and returns. A client submits serialized inputs to the backend, not directly to a worker. An accepted submission is not a completed workflow; await its typed result when completion matters. Stop polling and drain workers before closing storage. The introductory programs demonstrate that full lifecycle, including cleanup after errors.

## Feature map

| Topic | Samples |
| --- | --- |
| Basic execution and typed arguments | [simple](simple/), [complex parameters](complex-parameters/) |
| Registration names | [workflows](workflow-registration/), [activities](activity-registration/) |
| Concurrency | [workflow goroutines](concurrent/), [error groups](concurrent-errgroup/) |
| Child workflows and execution rollover | [subworkflows](subworkflow/), [continue as new](continue-as-new/) |
| External input and waiting | [signals](signal/), [child signals](signal-subworkflow/), [timers](timer/) |
| Failure handling | [errors](errors/), [retries](retries/), [cancellation](cancellation/) |
| Queues and separate processes | [queues](queues/), [split workers](simple-split-worker/), [scaling](scale/) |
| Serialization and context | [converter](converter/), [context propagation](context-propagation/) |
| Observability | [tracing](tracing/), [web diagnostics](web/), [workflow information](workflow-info/) |
| Application composition | [orchestrator](orchestrator/) |

## Running other demonstrations

Inspect each sample's entry point before running it. Some wait for an interrupt instead of exiting after the first result. Some start a loopback diagnostics server. Split-worker demonstrations need their producer and consumer processes running together.

The legacy shared helper in `runner.go` defaults to local Redis and contains development connection settings. Its recreation mode calls Redis `FlushAll`, which deletes data across databases on that Redis server. Never run those demonstrations against shared or production storage. Prefer an isolated in-memory or file-backed backend where the sample supports it.

From `make shell`, run a single-process feature demo with its documented command, for example `go run ./samples/concurrent -backend memory`. Split-process samples use `-backend sqlite` in both shells with the same working directory. In-memory storage cannot connect separate processes. Multi-host workers need a shared PostgreSQL, MySQL or Redis backend instead of the demo's local file.

The tracing sample also needs an OTLP/HTTP collector on port 4318 for export. Web diagnostics deliberately use slow activities and currently report a client timeout before those activities finish. The retries demo uses process-local counters to simulate failures in one instance; those counters are not a durable or concurrent application design.

Use `make shell` to enter the development toolchain for manual demonstrations. Production applications should construct their own backend and configuration rather than import the sample helper. See [architecture](../docs/architecture.md), [workflow behavior](../docs/guide.md) and [backend operations](../docs/backends.md).
