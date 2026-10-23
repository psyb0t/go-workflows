package sqlite

import (
	"context"
	"database/sql"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/workflow"
)

func scheduleActivity(ctx context.Context, tx *sql.Tx, queue workflow.Queue, instance *workflow.Instance, event *history.Event) error {
	// Attributes are already persisted via the history, we do not need to add them again.

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO activities
			(id, queue, instance_id, execution_id, event_type, timestamp, schedule_event_id, visible_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.ID,
		string(queue),
		instance.InstanceID,
		instance.ExecutionID,
		event.Type,
		event.Timestamp,
		event.ScheduleEventID,
		event.VisibleAt,
	); err != nil {
		return ctxerrors.Wrap(err, "inserting events")
	}

	return nil
}
