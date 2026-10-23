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
