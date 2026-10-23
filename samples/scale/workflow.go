// Each instance runs two activities sequentially. Throughput comes from many
// instances and worker replicas, not concurrent branches inside this workflow.
package simple_split_worker

import (
	"context"
	"fmt"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/workflow"
)

func Workflow1(ctx workflow.Context, msg string) (string, error) {
	logger := workflow.Logger(ctx)
	logger.Debug("Entering Workflow1", "msg", msg)
	defer logger.Debug("Leaving Workflow1")

	r1, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity1, 35, 12).Get(ctx)
	if err != nil {
		return "", ctxerrors.Wrap(err, "getting activity 1 result")
	}

	r2, err := workflow.ExecuteActivity[int](ctx, workflow.DefaultActivityOptions, Activity2).Get(ctx)
	if err != nil {
		return "", ctxerrors.Wrap(err, "getting activity 2 result")
	}

	return fmt.Sprintf("%s-%d,%d", msg, r1, r2), nil
}

func Activity1(ctx context.Context, a, b int) (int, error) {
	return a + b, nil
}

func Activity2(ctx context.Context) (int, error) {
	return 12, nil
}
