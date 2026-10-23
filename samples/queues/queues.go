package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"

	"github.com/google/uuid"
)

var (
	CustomActivityQueue = workflow.Queue("custom-activity-queue")
	CustomWorkflowQueue = workflow.Queue("custom-workflow-queue")
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	b := samples.GetBackend("queues", true, backend.WithLogger(slog.Default()))

	db, ok := b.(diag.Backend)
	if !ok {
		panic("backend does not implement diag.Backend")
	}

	// Start diagnostic server under /diag
	m := http.NewServeMux()
	m.Handle("/diag/", http.StripPrefix("/diag", diag.NewServeMux(db)))
	server := &http.Server{Addr: "127.0.0.1:3000", Handler: m, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	// Run worker
	w := RunDefaultWorker(ctx, b)

	if err := w.RegisterWorkflow(Workflow1); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(SubWorkflow); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(Activity1); err != nil {
		panic(err)
	}

	// This worker won't actually execute Activity2, but it still needs to be aware of its signature
	// since the workflow processed by this worker will schedule it.
	if err := w.RegisterActivity(Activity2); err != nil {
		panic(err)
	}

	activityWorker := worker.NewActivityWorker(b, &worker.ActivityWorkerOptions{
		ActivityPollers:          1,
		MaxParallelActivityTasks: 1,
		ActivityQueues:           []workflow.Queue{CustomActivityQueue},
	})

	if err := activityWorker.RegisterActivity(Activity2); err != nil {
		panic(err)
	}

	if err := activityWorker.Start(ctx); err != nil {
		panic(err)
	}

	workflowWorker := worker.NewWorkflowWorker(b, &worker.WorkflowWorkerOptions{
		WorkflowPollers:          1,
		MaxParallelWorkflowTasks: 1,
		WorkflowQueues:           []workflow.Queue{CustomWorkflowQueue},
	})

	if err := workflowWorker.RegisterWorkflow(SubWorkflow); err != nil {
		panic(err)
	}
	if err := workflowWorker.RegisterActivity(Activity2); err != nil {
		panic(err)
	}

	if err := workflowWorker.Start(ctx); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}

	// Start workflow via client
	c := client.New(b)

	runWorkflow(ctx, c)

	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint

	cancel()

	if err := w.WaitForCompletion(); err != nil {
		panic("could not stop worker" + err.Error())
	}

	if err := activityWorker.WaitForCompletion(); err != nil {
		panic("could not stop activity worker" + err.Error())
	}
}

func runWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "Hello world"+uuid.NewString(), 42, Inputs{
		Msg:   "",
		Times: 0,
	})
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

func RunDefaultWorker(ctx context.Context, mb backend.Backend) *worker.Worker {
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
