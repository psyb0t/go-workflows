package redis

import (
	"context"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend"
)

func (rb *redisBackend) GetStats(ctx context.Context) (*backend.Stats, error) {
	var err error

	s := &backend.Stats{}

	// get workflow instances
	activeInstances, err := rb.rdb.SCard(ctx, rb.keys.instancesActive()).Result()
	if err != nil {
		return nil, ctxerrors.Wrap(err, "getting active instances")
	}

	s.ActiveWorkflowInstances = activeInstances

	// get pending workflow tasks
	pendingWorkflows, err := rb.workflowQueue.Size(ctx, rb.rdb)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "getting active workflows")
	}

	s.PendingWorkflowTasks = pendingWorkflows

	// get pending activities
	pendingActivities, err := rb.activityQueue.Size(ctx, rb.rdb)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "getting active activities")
	}

	s.PendingActivityTasks = pendingActivities

	return s, nil
}
