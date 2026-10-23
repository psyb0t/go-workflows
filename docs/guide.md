# Workflow guide

## Execution and replay

A client persists a workflow-start event. A worker claims the task, replays recorded events and runs new commands until the workflow blocks or completes. Activities execute outside replay and their results become history events. Durable timers and signals resume waiting workflows.

The workflow function takes `workflow.Context` first and returns `error` or `(result, error)`. An activity returns the same shapes and may optionally take `context.Context` first. Register functions before starting a worker. Assign explicit names with `registry.WithName`; use the same names on producers and workers.

Use `worker.New` to process workflows and activities together, or `NewWorkflowWorker` and `NewActivityWorker` in separate processes. Configure queues and concurrency through worker options. Cancel the context passed to `Start` and call `WaitForCompletion` before closing storage.

## Determinism

The same history must produce the same workflow commands. Use `workflow.Now`, `ScheduleTimer`, `Sleep`, `Go`, workflow channels and `Select` instead of their native Go equivalents. Sort map keys before iteration affects decisions. Keep I/O, random values and wall-clock reads in activities. `workflow.SideEffect` records a serializable value for later replay.

Do not change an active workflow's ordering or the meaning of recorded commands. Deploy incompatible definitions under a new registered name and let old workers finish existing instances. `ContinueAsNew` starts a fresh execution with new arguments, reducing history growth; it does not make arbitrary incompatible code changes safe.

## Activities and retries

`workflow.ExecuteActivity[T](ctx, options, activity, args...).Get(ctx)` waits for a typed result. Set retry policy on `ActivityOptions.RetryOptions`; defaults are defined by `workflow.DefaultRetryOptions`. Each attempt can run again after failures or expired locks, so external effects must be idempotent.

Wrap failures with `ctxerrors.Wrap` at call boundaries. Inspect live error chains with `errors.Is` and `errors.As`. Mark failures that must not retry with `workflow.NewPermanentError`. Persisted failures retain messages, stacks, retry status and cause trees. Known panic and cancellation identities are restored; arbitrary application error types are not recreated automatically from JSON.

Retries stop for cancellation or permanent failures. Configure maximum attempts and backoff explicitly for expensive or rate-limited activities. A client result timeout stops waiting; it does not cancel the durable workflow.

## Subworkflows, signals and timers

`workflow.CreateSubWorkflowInstance[T]` starts a child and returns its future. Register children on workers as well as parents. Use a stable instance ID when another process must address an instance.

`client.SignalWorkflow` sends a serializable payload. A workflow receives it through `workflow.NewSignalChannel[T]`. Signals are stored before delivery; design payloads as versioned application contracts. See [signal](../samples/signal/) and [signal-subworkflow](../samples/signal-subworkflow/).

`workflow.ScheduleTimer` records a durable wakeup and returns a future. `workflow.Sleep` is its convenience form. Timer cancellation is cooperative through workflow contexts. See [timer](../samples/timer/) and [cancellation](../samples/cancellation/).

## Scope and logging

Seed request or job identity with `ctxscope.Set`. Default backend propagators serialize scope into workflow metadata and carry it through child workflows and activities. Values must be JSON primitives. Process-global scope remains local to its process.

Use `workflow.Logger(ctx)` inside workflows so replay does not print old records again. Activities can use `activity.Logger(ctx)`. Activity contexts include workflow/run/activity identity, type, attempt and queue. `backend.WithLogger` accepts the application's configured `slog` sink. The library does not replace the process-global logger. Configure JSON output, source fields and secret redaction in the host application.

Metadata and history are persisted data. Do not put credentials, access tokens or personal data in log scope, inputs or errors. Protect backend access and diagnostics.

## Tests

Use `tester.NewWorkflowTester[T]` for deterministic tests with activity expectations and virtual time. It does not replace backend integration tests. `activitytester` tests activities in a prepared activity context. [Samples](../samples/) include tests for retries, timers, subworkflows and converters.
