// Producer process. Start the matching worker, then run from a second make shell:
// go run ./samples/simple-split-worker/starter -backend sqlite
// Both processes must open the same simple-split.sqlite file. This producer exits
// after enqueueing; the worker continues processing without a producer connection.
package main

import (
	"context"
	"log"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	simple_split_worker "github.com/psyb0t/go-workflows/samples/simple-split-worker"
)

func main() {
	ctx := context.Background()

	b := samples.GetBackend("simple-split", false)

	// The client writes tasks to the shared backend; it does not dial the worker.
	c := client.New(b)
	startWorkflow(ctx, c)
}

func startWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, simple_split_worker.Workflow1, "Hello world "+uuid.NewString())
	if err != nil {
		panic("could not start workflow")
	}

	log.Println("Started workflow", wf.InstanceID)
}
