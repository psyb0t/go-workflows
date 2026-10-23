package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/samples"
	simple_split_worker "github.com/psyb0t/go-workflows/samples/simple-split-worker"
	"github.com/psyb0t/go-workflows/worker"
)

func main() {
	ctx := context.Background()

	b := samples.GetBackend("simple-split", false)

	// Run worker
	go RunWorker(ctx, b)

	c2 := make(chan os.Signal, 1)
	signal.Notify(c2, os.Interrupt)
	<-c2
}

func RunWorker(ctx context.Context, mb backend.Backend) {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(simple_split_worker.Workflow1); err != nil {
		panic(err)
	}

	if err := w.RegisterActivity(simple_split_worker.Activity1); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(simple_split_worker.Activity2); err != nil {
		panic(err)
	}

	if err := w.Start(ctx); err != nil {
		panic("could not start worker")
	}
}
