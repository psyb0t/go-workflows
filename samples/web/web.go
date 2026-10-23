// Run from make shell with: go run ./samples/web -backend memory.
// The diagnostics mux listens on 127.0.0.1:3000/diag/ and reads the worker's
// backend. It is not a task server. Activities intentionally take 50 seconds,
// longer than the client's ten-second wait, so this demo reports a timeout.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/samples"

	"github.com/psyb0t/go-workflows/worker"

	"github.com/google/uuid"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	b := samples.GetBackend("web", true)

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
			log.Printf("diagnostics server failed: %v", err)
		}
	}()

	// Run worker
	w := RunWorker(ctx, b)

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
}

func runWorkflow(ctx context.Context, c *client.Client) {
	wf, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "Hello world"+uuid.NewString(), 2, Inputs{
		Msg:   "",
		Times: 2,
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

func RunWorker(ctx context.Context, mb backend.Backend) *worker.Worker {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(Workflow1); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(RunJob); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(RunStep); err != nil {
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
