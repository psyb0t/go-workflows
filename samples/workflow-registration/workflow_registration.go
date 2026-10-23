package main

import (
	"context"
	"log"
	"time"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/registry"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"

	"github.com/google/uuid"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	b := samples.GetBackend("simple", true)

	// Run worker
	w := RunWorker(ctx, b)

	// Start workflow via client
	c := client.New(b)

	runWorkflow(ctx, c)

	cancel()

	if err := w.WaitForCompletion(); err != nil {
		panic("could not stop worker" + err.Error())
	}
}

func runWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, "WorkflowName", "Hello world")
	if err != nil {
		log.Fatal(err)
		panic("could not start workflow")
	}

	result, err := client.GetWorkflowResult[int](ctx, c, wf, time.Second*10)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Workflow finished. Result:", result)
}

func RunWorker(ctx context.Context, mb backend.Backend) *worker.Worker {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(Workflow1, registry.WithName("WorkflowName")); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(SubWorkflow1, registry.WithName("SubWorkflowName")); err != nil {
		panic(err)
	}

	if err := w.RegisterActivity(Activity1); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}

	return w
}
