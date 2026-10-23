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

	// Register workflows and activities explicitly
	if err := orchestrator.RegisterWorkflow(SimpleWorkflow); err != nil {
		panic(err)
	}
	if err := orchestrator.RegisterActivity(ProcessMessage); err != nil {
		panic(err)
	}

	if err := orchestrator.Start(ctx); err != nil {
		panic("could not start orchestrator: " + err.Error())
	}

	// Create instance ID
	instanceID := uuid.NewString()

	// Create and run workflow using the orchestrator
	instance, err := orchestrator.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: instanceID,
	}, SimpleWorkflow, "Hello from orchestrator!")
	if err != nil {
		panic("could not create workflow instance: " + err.Error())
	}

	// Wait for result using client package directly
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

// SimpleWorkflow is a basic workflow that calls an activity and returns its result
func SimpleWorkflow(ctx workflow.Context, message string) (string, error) {
	f := workflow.ExecuteActivity[string](ctx, workflow.DefaultActivityOptions, ProcessMessage, message)

	result, err := f.Get(ctx)
	if err != nil {
		return "", ctxerrors.Wrap(err, "execute main")
	}

	return result, nil
}

// ProcessMessage is a simple activity that processes a message
func ProcessMessage(ctx context.Context, message string) (string, error) {
	return message + " (processed by activity)", nil
}
