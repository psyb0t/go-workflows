// Run from make shell with: go run ./samples/orchestrator -backend memory.
// The orchestrator bundles a client and worker on one backend. It registers
// definitions, processes a message through an activity, drains and exits.
package main

import (
	"context"
	"log"
	"time"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"

	"github.com/google/uuid"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend := samples.GetBackend("orchestrator", true, backend.WithWorkerName("orchestrator-worker"))

	orchestrator := worker.NewWorkflowOrchestrator(
		backend,
		nil,
	)

	// The embedded client can submit work only after a worker knows these definitions.
	if err := orchestrator.RegisterWorkflow(SimpleWorkflow); err != nil {
		panic(err)
	}
	if err := orchestrator.RegisterActivity(ProcessMessage); err != nil {
		panic(err)
	}

	if err := orchestrator.Start(ctx); err != nil {
		panic("could not start orchestrator: " + err.Error())
	}

	// Generate nondeterministic identifiers outside replayed workflow functions.
	instanceID := uuid.NewString()

	// Submission still goes through backend storage, even with this convenience wrapper.
	instance, err := orchestrator.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: instanceID,
	}, SimpleWorkflow, "Hello from orchestrator!")
	if err != nil {
		panic("could not create workflow instance: " + err.Error())
	}

	// The result API works with the orchestrator's ordinary embedded client.
	result, err := client.GetWorkflowResult[string](ctx, orchestrator.Client, instance, 10*time.Second)
	if err != nil {
		panic("error getting workflow result: " + err.Error())
	}

	log.Printf("Workflow result: %s\n", result)

	// Clean shutdown
	cancel()

	if err := orchestrator.WaitForCompletion(); err != nil {
		panic("could not stop orchestrator: " + err.Error())
	}
}

// SimpleWorkflow awaits a recorded activity result, which is reused during replay.
func SimpleWorkflow(ctx workflow.Context, message string) (string, error) {
	f := workflow.ExecuteActivity[string](ctx, workflow.DefaultActivityOptions, ProcessMessage, message)

	result, err := f.Get(ctx)
	if err != nil {
		return "", ctxerrors.Wrap(err, "execute main")
	}

	return result, nil
}

// ProcessMessage is ordinary Go code. External I/O would belong here, not in SimpleWorkflow.
func ProcessMessage(ctx context.Context, message string) (string, error) {
	return message + " (processed by activity)", nil
}
