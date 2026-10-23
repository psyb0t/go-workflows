package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/samples"
	scale "github.com/psyb0t/go-workflows/samples/scale"
	"github.com/psyb0t/go-workflows/worker"
)

func main() {
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())

	b := samples.GetBackend("scale", false)

	// Run worker
	go RunWorker(ctx, b)

	c2 := make(chan os.Signal, 1)
	signal.Notify(c2, os.Interrupt)
	<-c2

	log.Println("Shutting down")
	cancel()
}

func RunWorker(ctx context.Context, mb backend.Backend) {
	w := worker.New(mb, &worker.Options{
		WorkflowWorkerOptions: worker.WorkflowWorkerOptions{
			WorkflowPollers:          1,
			MaxParallelWorkflowTasks: 100,
		},
		ActivityWorkerOptions: worker.ActivityWorkerOptions{
			ActivityPollers:          1,
			MaxParallelActivityTasks: 100,
		},
	})

	if err := w.RegisterWorkflow(scale.Workflow1); err != nil {
		panic(err)
	}

	if err := w.RegisterActivity(scale.Activity1); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(scale.Activity2); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}
}
