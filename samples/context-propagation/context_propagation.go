// Run from make shell with: go run ./samples/context-propagation -backend memory.
// The propagator serializes selected context values into persisted task metadata.
// Activities read the values restored by the worker; the process exits afterward.
package main

import (
	"context"
	"log"
	"time"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"

	"github.com/google/uuid"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	b := samples.GetBackend("simple", true, backend.WithContextPropagator(&myPropagator{}))

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
	ctx = withMyValues(ctx, &myData{
		Name:  "test",
		Count: 2,
	})

	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1)
	if err != nil {
		log.Fatal(err)
		panic("could not start workflow")
	}

	result, err := client.GetWorkflowResult[string](ctx, c, wf, time.Second*60)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Workflow finished. Result:", result)
}

func RunWorker(ctx context.Context, mb backend.Backend) *worker.Worker {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(Workflow1); err != nil {
		panic(err)
	}

	if err := w.RegisterActivity(Activity1); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(Activity2); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}

	return w
}
