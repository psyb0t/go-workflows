// Run from make shell with: go run ./samples/signal -backend memory.
// The client sends two named signals to the parent and one to its child.
// Signal values are persisted even if the workflow has not received them yet.
// After the child returns, the process waits for Ctrl+C.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := context.Background()

	b := samples.GetBackend("signal", true)

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
	subID := uuid.NewString()

	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "Hello world", subID)
	if err != nil {
		panic("could not start workflow")
	}

	log.Println("Started workflow", wf.InstanceID)

	// Client-side sleeps only space out the demonstration. Workflow waits use
	// replay-safe signal channels, not native Go channels or time.Sleep.
	time.Sleep(2 * time.Second)
	if err := c.SignalWorkflow(ctx, wf.InstanceID, "test", 42); err != nil {
		panic(err)
	}

	time.Sleep(2 * time.Second)
	if err := c.SignalWorkflow(ctx, wf.InstanceID, "test2", 42); err != nil {
		panic(err)
	}

	time.Sleep(2 * time.Second)
	if err := c.SignalWorkflow(ctx, subID, "sub-signal", 42); err != nil {
		panic(err)
	}

	log.Println("Signaled workflow", wf.InstanceID)
}

func RunWorker(ctx context.Context, mb backend.Backend) {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(Workflow1); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(SubWorkflow1); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}
}

func Workflow1(ctx workflow.Context, msg string, subID string) (string, error) {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering Workflow1")

	logger.Debug("Waiting for first signal")
	workflow.Select(ctx,
		workflow.Receive(workflow.NewSignalChannel[int](ctx, "test"), func(ctx workflow.Context, r int, ok bool) {
			logger.Debug("Received signal:", "r", r)
		}),
	)

	logger.Debug("Waiting for second signal")
	workflow.NewSignalChannel[int](ctx, "test2").Receive(ctx)
	logger.Debug("Received second signal")

	if _, err := workflow.CreateSubWorkflowInstance[any](ctx, workflow.SubWorkflowOptions{
		InstanceID: subID,
	}, SubWorkflow1).Get(ctx); err != nil {
		panic(err)
	}

	logger.Debug("Sub workflow finished")

	return "result", nil
}

func SubWorkflow1(ctx workflow.Context) (string, error) {
	logger := workflow.Logger(ctx)
	logger.Debug("Waiting for signal from sub-worflow")

	c := workflow.NewSignalChannel[int](ctx, "sub-signal")
	c.Receive(ctx)

	logger.Debug("Received sub-workflow signal")

	return "World", nil
}
