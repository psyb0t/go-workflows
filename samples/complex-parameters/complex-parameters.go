// Run from make shell with: go run ./samples/complex-parameters -backend memory.
// Exported struct fields become persisted workflow inputs through the converter.
// The worker runs two activities; the process then waits for Ctrl+C.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := context.Background()

	b := samples.GetBackend("complex-parameters", true)

	// Run worker
	go RunWorker(ctx, b)

	// Start workflow via client
	c := client.New(b)

	startWorkflow(ctx, c)

	c2 := make(chan os.Signal, 1)
	signal.Notify(c2, os.Interrupt)
	<-c2
}

func startWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, Workflow1Args{
		Name: "John",
		Age:  35,
	})
	if err != nil {
		log.Fatal(err)
		panic("could not start workflow")
	}

	log.Println("Started workflow", wf.InstanceID)
}

func RunWorker(ctx context.Context, mb backend.Backend) {
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
}

// Workflow1Args must remain compatible with inputs already saved in history.
type Workflow1Args struct {
	Name string
	Age  int
}

func Workflow1(ctx workflow.Context, args Workflow1Args) error {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering Workflow1", "args.name", args.Name, "args.age", args.Age)
	defer logger.Debug("Leaving Workflow1")

	r1, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12).Get(ctx)
	if err != nil {
		panic("error getting activity 1 result")
	}
	logger.Debug("R1 result:", "r1", r1)

	r2, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity2).Get(ctx)
	if err != nil {
		panic("error getting activity 1 result")
	}
	logger.Debug("R2 result:", "r2", r2)

	return nil
}

func Activity1(ctx context.Context, a, b int) (int, error) {
	log.Println("Entering Activity1")

	defer func() {
		log.Println("Leaving Activity1")
	}()

	return a + b, nil
}

func Activity2(ctx context.Context) (int, error) {
	log.Println("Entering Activity2")

	defer func() {
		log.Println("Leaving Activity2")
	}()

	return 12, nil
}
