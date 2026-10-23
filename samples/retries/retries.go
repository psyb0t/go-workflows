package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := context.Background()

	b := samples.GetBackend("retries", true)

	// Run worker
	go RunWorker(ctx, b)

	// Start workflow via client
	c := client.New(b)

	startWorkflow(ctx, c)
	// startWorkflow(ctx, c)

	c2 := make(chan os.Signal, 1)
	signal.Notify(c2, os.Interrupt)
	<-c2
}

func startWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "Hello world"+uuid.NewString())
	if err != nil {
		panic("could not start workflow")
	}

	log.Println("Started workflow", wf.InstanceID)
}

func RunWorker(ctx context.Context, mb backend.Backend) {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(Workflow1); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(WorkflowWithFailures); err != nil {
		panic(err)
	}

	if err := w.RegisterActivity(Activity1); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}
}

func Workflow1(ctx workflow.Context, msg string) error {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering Workflow1", "msg", msg)
	defer logger.Debug("Leaving Workflow1")

	// Illustrate sub workflow retries. The called workflow will fail a few times, and its execution will be retried.
	_, err := workflow.CreateSubWorkflowInstance[any](ctx, workflow.SubWorkflowOptions{
		RetryOptions: workflow.RetryOptions{
			MaxAttempts:        3,
			FirstRetryInterval: time.Second * 3,
			BackoffCoefficient: 1,
		},
	}, WorkflowWithFailures, "Hello world"+uuid.NewString()).Get(ctx)
	if err != nil {
		return ctxerrors.Wrap(err, "starting subworkflow")
	}

	logger.Debug("Completing workflow 1")
	return nil
}

var workflowCalls = 0

func WorkflowWithFailures(ctx workflow.Context, msg string) error {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering WorkflowWithFailures", "msg", msg)
	defer logger.Debug("Leaving WorkflowWithFailures")

	workflowCalls++
	if workflowCalls < 3 {
		return ctxerrors.Wrapf(commerr.ErrInvalidState, "workflow error %d", workflowCalls)
	}

	// Illustrate activity retries. The called activity will fail a few times, and its execution will be retried.
	r1, err := workflow.ExecuteActivity[int](ctx, workflow.ActivityOptions{
		RetryOptions: workflow.RetryOptions{
			MaxAttempts:        3,
			FirstRetryInterval: time.Second * 3,
			BackoffCoefficient: 2,
		},
	}, Activity1, 35).Get(ctx)
	if err != nil {
		logger.Error("Error from Activity 1", "err", err)
		return ctxerrors.Wrap(err, "getting result from activity 1")
	}

	logger.Debug("R1 result", "r1", r1)

	logger.Debug("Completing workflow with failures")
	return nil
}

var activityCalls = 0

func Activity1(ctx context.Context, a int) (int, error) {
	log.Println("Entering Activity1")
	defer log.Println("Leaving Activity1")

	activityCalls++
	if activityCalls < 3 {
		time.Sleep(2 * time.Second)

		log.Println(">> Activity error", activityCalls)
		return 0, ctxerrors.Wrapf(commerr.ErrInvalidState, "activity error %d", activityCalls)
	}

	return a, nil
}
