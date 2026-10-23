package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/psyb0t/ctxerrors"

	"github.com/google/uuid"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/samples"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func main() {
	ctx := context.Background()

	b := samples.GetBackend("errors", true)

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
			slog.Error("diagnostics server failed", "err", err)
		}
	}()

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
	_, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: uuid.NewString(),
	}, Workflow1, "Hello world"+uuid.NewString())
	if err != nil {
		panic("could not start workflow")
	}
}

func RunWorker(ctx context.Context, mb backend.Backend) {
	w := worker.New(mb, nil)

	if err := w.RegisterWorkflow(Workflow1); err != nil {
		panic(err)
	}
	if err := w.RegisterWorkflow(SubWorkflow); err != nil {
		panic(err)
	}

	if err := w.RegisterActivity(GenericErrorActivity); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(PanicActivity); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(CustomErrorActivity); err != nil {
		panic(err)
	}
	if err := w.RegisterActivity(WrappedErrorActivity); err != nil {
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

	actOptions := workflow.DefaultActivityOptions
	actOptions.RetryOptions = workflow.RetryOptions{
		MaxAttempts: 1,
	}

	_, err := workflow.ExecuteActivity[int](ctx, actOptions, GenericErrorActivity).Get(ctx)
	if err != nil {
		handleError(ctx, "GenericError", logger, err)
	}

	_, err = workflow.ExecuteActivity[int](ctx, actOptions, CustomErrorActivity).Get(ctx)
	if err != nil {
		handleError(ctx, "CustomError", logger, err)
	}

	_, err = workflow.ExecuteActivity[int](ctx, actOptions, PanicActivity).Get(ctx)
	if err != nil {
		handleError(ctx, "Panic", logger, err)
	}

	_, err = workflow.ExecuteActivity[int](ctx, actOptions, WrappedErrorActivity).Get(ctx)
	if err != nil {
		handleError(ctx, "Wrapped", logger, err)
	}

	wfOptions := workflow.DefaultSubWorkflowOptions
	wfOptions.RetryOptions = workflow.RetryOptions{
		MaxAttempts: 1,
	}
	_, err = workflow.CreateSubWorkflowInstance[int](ctx, wfOptions, SubWorkflow).Get(ctx)
	if err != nil {
		handleError(ctx, "SubWorkflow", logger, err)
	}

	return nil
}

func handleError(ctx workflow.Context, name string, logger *slog.Logger, err error) {
	logger = logger.With("activity", name)

	var werr *workflow.Error
	if errors.As(err, &werr) {
		switch werr.Type {
		case "CustomError":
			logger.Error("Custom error", "err", werr)
			return
		}

		logger.Error("Generic workflow error", "err", werr, "stack", werr.Stack())
		return
	}

	var perr *workflow.PanicError
	if errors.As(err, &perr) {
		logger.Error("Panic", "err", perr, "stack", perr.Stack())
		return
	}

	logger.Error("Generic error", "err", err)
}

func GenericErrorActivity(ctx context.Context) (int, error) {
	return 0, ctxerrors.New("some activity error")
}

type CustomError struct {
	msg string
}

func (e *CustomError) Error() string {
	return e.msg
}

// Ensure CustomError implements the error interface
var _ error = (*CustomError)(nil)

func CustomErrorActivity(ctx context.Context) (int, error) {
	return 0, &CustomError{msg: "some custom error"}
}

func PanicActivity(ctx context.Context) (int, error) {
	return someFunc(), nil
}

func someFunc() int {
	panic("panic!")
}

func WrappedErrorActivity(ctx context.Context) (int, error) {
	return 0, ctxerrors.Wrap(ctxerrors.New("inner error"), "wrapped error")
}

func SubWorkflow(ctx workflow.Context) (int, error) {
	panic("swf panic!")
}
