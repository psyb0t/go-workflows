package mysql

import (
	"context"
	"database/sql"
	"time"

	"github.com/psyb0t/go-workflows/internal/cleanup"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/workflow"
)

func (mb *mysqlBackend) GetStats(ctx context.Context) (*backend.Stats, error) {
	s := &backend.Stats{}

	tx, err := mb.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return nil, ctxerrors.Wrap(err, "failed to start transaction")
	}
	defer cleanup.Rollback(ctx, tx)

	// Get active instances
	row := tx.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM instances i WHERE i.completed_at IS NULL",
	)
	if err := row.Err(); err != nil {
		return nil, ctxerrors.Wrap(err, "failed to query active instances")
	}

	var activeInstances int64
	if err := row.Scan(&activeInstances); err != nil {
		return nil, ctxerrors.Wrap(err, "failed to scan active instances")
	}

	s.ActiveWorkflowInstances = activeInstances

	// Get workflow instances ready to be picked up
	now := time.Now()
	workflowRows, err := tx.QueryContext(
		ctx,
		`SELECT i.queue, COUNT(*)
			FROM instances i
			INNER JOIN pending_events pe ON i.instance_id = pe.instance_id
			WHERE
				i.state = ? AND i.completed_at IS NULL
				AND (pe.visible_at IS NULL OR pe.visible_at <= ?)
				AND (i.locked_until IS NULL OR i.locked_until < ?)
			GROUP BY i.queue`,
		core.WorkflowInstanceStateActive,
		now, // event.visible_at
		now, // locked_until
	)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "failed to query active instances")
	}

	s.PendingWorkflowTasks = make(map[core.Queue]int64)

	for workflowRows.Next() {
		var queue string
		var pendingInstances int64
		if err := workflowRows.Scan(&queue, &pendingInstances); err != nil {
			return nil, ctxerrors.Wrap(err, "failed to scan active instances")
		}

		s.PendingWorkflowTasks[workflow.Queue(queue)] = pendingInstances
	}

	if err := workflowRows.Err(); err != nil {
		return nil, ctxerrors.Wrap(err, "failed to read active instances")
	}

	// Get pending activities
	activityRows, err := tx.QueryContext(
		ctx,
		"SELECT queue, COUNT(*) FROM activities GROUP BY queue")
	if err != nil {
		return nil, ctxerrors.Wrap(err, "failed to query active activities")
	}

	s.PendingActivityTasks = make(map[core.Queue]int64)

	for activityRows.Next() {
		var queue string
		var pendingActivities int64
		if err := activityRows.Scan(&queue, &pendingActivities); err != nil {
			return nil, ctxerrors.Wrap(err, "failed to scan active activities")
		}

		s.PendingActivityTasks[workflow.Queue(queue)] = pendingActivities
	}

	if err := activityRows.Err(); err != nil {
		return nil, ctxerrors.Wrap(err, "failed to read active activities")
	}

	return s, nil
}
