package main

import (
	"context"
	"time"

	"github.com/psyb0t/ctxerrors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/psyb0t/go-workflows/activity"
	"github.com/psyb0t/go-workflows/workflow"
)

type Inputs struct {
	Msg   string
	Times int
}

func Workflow1(ctx workflow.Context, msg string, times int, inputs Inputs) (int, error) {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering Workflow1", "msg", msg, "times", times, "inputs", inputs)
	defer logger.Debug("Leaving Workflow1")

	tracer := workflow.Tracer(ctx)
	ctx, span := tracer.Start(ctx, "Workflow1 custom span")
	defer span.End()

	_, customSpan := tracer.Start(ctx, "Workflow1 custom inner span", trace.WithAttributes(
		// Add additional
		attribute.String("msg", "hello world"),
	))

	// Do something

	customSpan.End()

	if _, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12).Get(ctx); err != nil {
		panic(err)
	}

	if err := workflow.Sleep(ctx, time.Second*1); err != nil {
		panic(err)
	}

	if _, err := workflow.CreateSubWorkflowInstance[any](ctx, workflow.DefaultSubWorkflowOptions, Subworkflow).Get(ctx); err != nil {
		panic(err)
	}

	if _, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12).Get(ctx); err != nil {
		panic(err)
	}

	_, span = workflow.Tracer(ctx).Start(ctx, "Wait-for-signal")
	workflow.NewSignalChannel[string](ctx, "test-signal").Receive(ctx)
	span.End()

	r1, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, RetriedActivity, 35, 12).Get(ctx)
	if err != nil {
		return r1, ctxerrors.Wrap(err, "execute workflow")
	}

	return r1, nil
}

func Subworkflow(ctx workflow.Context) error {
	if err := workflow.Sleep(ctx, time.Millisecond*500); err != nil {
		panic(err)
	}

	if _, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12).Get(ctx); err != nil {
		panic(err)
	}

	return nil
}

func Activity1(ctx context.Context, a, b int) (int, error) {
	logger := activity.Logger(ctx)
	logger.Debug("Entering Activity1")
	defer logger.Debug("Leaving Activity1")

	ctx, span := otel.Tracer("activity").Start(ctx, "Custom Activity1 span")
	defer span.End()

	_, span = otel.Tracer("activity").Start(ctx, "Another one")
	defer span.End()

	time.Sleep(300 * time.Millisecond)

	return a + b, nil
}

func RetriedActivity(ctx context.Context, a, b int) (int, error) {
	logger := activity.Logger(ctx)

	if activity.Attempt(ctx) < 1 {
		logger.Info("Simulating failure")
		return 0, ctxerrors.New("simulated failure")
	}

	return a + b, nil
}
