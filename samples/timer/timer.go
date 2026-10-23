// Run from make shell with: go run ./samples/timer -backend memory.
// Race a persisted two-second timer against a ten-second activity. The timer
// normally wins; if the activity wins, its branch cancels the timer. The demo exits.
package main

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/activity"
	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	b := samples.GetBackend("timer", true)

	// Run worker
	w := RunWorker(ctx, b)

	// Start workflow via client
	c := client.New(b)

	startWorkflow(ctx, c)

	cancel()
	if err := w.WaitForCompletion(); err != nil {
		panic(err)
	}
}

func startWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "Hello world")
	if err != nil {
		panic("could not start workflow")
	}

	result, err := client.GetWorkflowResult[string](ctx, c, wf, time.Second*15)
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

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}

	return w
}

func Workflow1(ctx workflow.Context, msg string) (string, error) {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering Workflow1, input: ", "msg", msg)
	defer logger.Debug("Leaving Workflow1")

	a1 := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12)

	tctx, cancel := workflow.WithCancel(ctx)

	// Select resumes the first ready branch. ScheduleTimer survives worker
	// restarts with a persistent backend; time.Sleep inside a workflow would not.
	workflow.Select(
		ctx,
		workflow.Await(workflow.ScheduleTimer(tctx, 2*time.Second), func(ctx workflow.Context, f workflow.Future[any]) {
			if _, err := f.Get(ctx); err != nil {
				logger.Debug("Timer canceled")
			} else {
				logger.Debug("Timer fired")
			}
		}),
		workflow.Await(a1, func(ctx workflow.Context, f workflow.Future[int]) {
			r, err := f.Get(ctx)
			if err != nil {
				panic(err)
			}

			logger.Debug("Activity result", "r", r)

			// Cancel timer
			cancel()
		}),
	)

	return "result", nil
}

func Activity1(ctx context.Context, a, b int) (int, error) {
	logger := activity.Logger(ctx)
	logger.Debug("Entering Activity1")

	time.Sleep(10 * time.Second)

	defer func() {
		logger.Debug("Leaving Activity1")
	}()

	return a + b, nil
}
