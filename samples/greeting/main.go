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
