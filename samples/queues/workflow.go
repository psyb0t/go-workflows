package main

import (
	"context"

	"github.com/psyb0t/go-workflows/activity"
	"github.com/psyb0t/go-workflows/workflow"
)

type Inputs struct {
	Msg   string
	Times int
}

func Workflow1(ctx workflow.Context, msg string, times int, inputs Inputs) (int, error) {
	logger := workflow.Logger(ctx)
	logger.Info("Entering Workflow1", "msg", msg, "times", times, "inputs", inputs)
	defer logger.Info("Leaving Workflow1")

	r1, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12).Get(ctx)
	if err != nil {
		panic("error getting activity 1 result")
	}
	logger.Info("R1 result", "r1", r1)

	// Only activity workers polling CustomActivityQueue can claim this task.
	r2, err := workflow.ExecuteActivity[int](ctx, workflow.ActivityOptions{
		Queue: CustomActivityQueue,
	}, Activity2).Get(ctx)
	if err != nil {
		panic("error getting activity 2 result")
	}
	logger.Info("R2 result", "r2", r2)

	// Routing a child workflow does not move its parent off the default queue.
	if _, err := workflow.CreateSubWorkflowInstance[any](ctx, workflow.SubWorkflowOptions{
		Queue: CustomWorkflowQueue,
	}, SubWorkflow).Get(ctx); err != nil {
		panic(err)
	}

	return r1 + r2, nil
}

func SubWorkflow(ctx workflow.Context) error {
	logger := workflow.Logger(ctx)
	logger.Info("Entering SubWorkflow")
	defer logger.Info("Leaving SubWorkflow")

	// A workflow queue and an activity queue are independent routing choices.
	r2, err := workflow.ExecuteActivity[int](ctx, workflow.ActivityOptions{
		Queue: CustomActivityQueue,
	}, Activity2).Get(ctx)
	if err != nil {
		panic("error getting activity 2 result")
	}
	logger.Info("R2 result", "r2", r2)

	return nil
}

func Activity1(ctx context.Context, a, b int) (int, error) {
	logger := activity.Logger(ctx)
	logger.Info("Entering Activity1")
	defer logger.Info("Leaving Activity1")

	return a + b, nil
}

func Activity2(ctx context.Context) (int, error) {
	logger := activity.Logger(ctx)
	logger.Info("Entering Activity2")
	defer logger.Info("Leaving Activity2")

	return 12, nil
}
