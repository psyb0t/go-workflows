package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/psyb0t/go-workflows/internal/cleanup"

	"github.com/psyb0t/ctxerrors"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/metadata"
	"github.com/psyb0t/go-workflows/backend/metrics"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/metrickeys"
	"github.com/psyb0t/go-workflows/internal/workflowerrors"
	"github.com/psyb0t/go-workflows/workflow"

	_ "modernc.org/sqlite"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed db/migrations/*.sql
var migrationsFS embed.FS

func NewInMemoryBackend(opts ...option) *sqliteBackend {
	// Use a unique named in-memory database
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_txlock=immediate", uuid.NewString())
	b := newSqliteBackend(dsn, opts...)

	b.db.SetConnMaxIdleTime(0)
	b.db.SetMaxIdleConns(1)

	// WORKAROUND: Keep a connection open at all times to prevent the in-memory db from being dropped
	b.db.SetMaxOpenConns(2)

	var err error
	b.memConn, err = b.db.Conn(context.Background())
	if err != nil {
		panic(err)
	}

	return b
}

func NewSqliteBackend(path string, opts ...option) *sqliteBackend {
	return newSqliteBackend(fmt.Sprintf("file:%v?_txlock=immediate", path), opts...)
}

func newSqliteBackend(dsn string, opts ...option) *sqliteBackend {
	options := &options{
		Options:         backend.ApplyOptions(),
		ApplyMigrations: true,
	}

	for _, opt := range opts {
		opt(options)
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		panic(err)
	}

	// Set WAL mode via PRAGMA
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		panic(err)
	}

	if _, err = db.Exec("PRAGMA busy_timeout = 5000;"); err != nil {
		panic(err)
	}

	// SQLite does not support multiple writers on the database, see https://www.sqlite.org/faq.html#q5
	// A frequently used workaround is to have a single connection, effectively acting as a mutex
	// See https://github.com/mattn/go-sqlite3/issues/274 for more context
	db.SetMaxOpenConns(1)

	b := &sqliteBackend{
		db:             db,
		workerName:     getWorkerName(options),
		options:        options,
		ownsConnection: true,
	}

	// Apply migrations
	if options.ApplyMigrations {
		if err := b.Migrate(); err != nil {
			panic(err)
		}
	}

	return b
}

// NewSqliteBackendWithDB creates a new SQLite backend using an existing database connection.
// When using this constructor, the backend will not close the database connection when Close() is called.
// Migrations can still be applied using WithApplyMigrations(true) as SQLite does not require
// special connection settings for migrations.
// Note: The caller is responsible for configuring the connection appropriately (e.g., WAL mode,
// busy timeout, max open connections).
func NewSqliteBackendWithDB(db *sql.DB, opts ...option) *sqliteBackend {
	options := &options{
		Options:         backend.ApplyOptions(),
		ApplyMigrations: false,
	}

	for _, opt := range opts {
		opt(options)
	}

	b := &sqliteBackend{
		db:             db,
		workerName:     getWorkerName(options),
		options:        options,
		ownsConnection: false,
	}

	if options.ApplyMigrations {
		if err := b.Migrate(); err != nil {
			panic(err)
		}
	}

	return b
}

type sqliteBackend struct {
	db             *sql.DB
	workerName     string
	options        *options
	ownsConnection bool

	memConn *sql.Conn
}

var _ backend.Backend = (*sqliteBackend)(nil)

func (sb *sqliteBackend) FeatureSupported(feature backend.Feature) bool {
	return true
}

func (sb *sqliteBackend) Close() error {
	if sb.memConn != nil {
		if err := sb.memConn.Close(); err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}
	}

	if !sb.ownsConnection {
		return nil
	}

	return sb.db.Close()
}

// Migrate applies any pending database migrations.
func (sb *sqliteBackend) Migrate() error {
	sb.options.Logger.Info("Applying migrations...")

	dbi, err := sqlite.WithInstance(sb.db, &sqlite.Config{})
	if err != nil {
		return ctxerrors.Wrap(err, "creating migration instance")
	}

	migrations, err := iofs.New(migrationsFS, "db/migrations")
	if err != nil {
		return ctxerrors.Wrap(err, "creating migration source")
	}

	m, err := migrate.NewWithInstance("iofs", migrations, "sqlite", dbi)
	if err != nil {
		return ctxerrors.Wrap(err, "creating migration")
	}

	if err := m.Up(); err != nil {
		if !errors.Is(err, migrate.ErrNoChange) {
			return ctxerrors.Wrap(err, "running migrations")
		}

		sb.options.Logger.Info("No migrations to apply")
	}

	return nil
}

func (sb *sqliteBackend) Metrics() metrics.Client {
	return sb.options.Metrics.WithTags(metrics.Tags{metrickeys.Backend: "sqlite"})
}

func (sb *sqliteBackend) Tracer() trace.Tracer {
	return sb.options.TracerProvider.Tracer(backend.TracerName)
}

func (sb *sqliteBackend) Options() *backend.Options {
	return sb.options.Options
}

func (sb *sqliteBackend) CreateWorkflowInstance(ctx context.Context, instance *workflow.Instance, event *history.Event) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "starting transaction")
	}
	defer cleanup.Rollback(ctx, tx)

	a := event.Attributes.(*history.ExecutionStartedAttributes)

	// Create workflow instance
	if err := createInstance(ctx, tx, a.Queue, instance, a.Metadata); err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}

	if err := insertPendingEvents(ctx, tx, instance, []*history.Event{event}); err != nil {
		return ctxerrors.Wrap(err, "inserting new event")
	}

	if err := tx.Commit(); err != nil {
		return ctxerrors.Wrap(err, "creating workflow instance")
	}

	return nil
}

func createInstance(ctx context.Context, tx *sql.Tx, queue workflow.Queue, wfi *workflow.Instance, metadata *workflow.Metadata) error {
	// Check for existing instance
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM `instances` WHERE id = ? AND state = ? LIMIT 1", wfi.InstanceID, core.WorkflowInstanceStateActive).
		Scan(new(int)); err != sql.ErrNoRows {
		return backend.ErrInstanceAlreadyExists
	}

	var parentInstanceID, parentExecutionID *string
	var parentEventID *int64
	if wfi.SubWorkflow() {
		parentInstanceID = &wfi.Parent.InstanceID
		parentExecutionID = &wfi.Parent.ExecutionID
		parentEventID = &wfi.ParentEventID
	}

	metadataJson, err := json.Marshal(metadata)
	if err != nil {
		return ctxerrors.Wrap(err, "marshaling metadata")
	}

	_, err = tx.ExecContext(
		ctx,
		"INSERT INTO `instances` (queue, id, execution_id, parent_instance_id, parent_execution_id, parent_schedule_event_id, metadata, state) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		string(queue),
		wfi.InstanceID,
		wfi.ExecutionID,
		parentInstanceID,
		parentExecutionID,
		parentEventID,
		string(metadataJson),
		core.WorkflowInstanceStateActive,
	)
	if err != nil {
		return ctxerrors.Wrap(err, "inserting workflow instance")
	}

	return nil
}

func (sb *sqliteBackend) RemoveWorkflowInstance(ctx context.Context, instance *core.WorkflowInstance) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	if err := sb.removeWorkflowInstance(ctx, instance, tx); err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}

	return tx.Commit()
}

func (sb *sqliteBackend) removeWorkflowInstance(ctx context.Context, instance *core.WorkflowInstance, tx *sql.Tx) error {
	instanceID := instance.InstanceID
	executionID := instance.ExecutionID

	row := tx.QueryRowContext(ctx, "SELECT state FROM `instances` WHERE id = ? AND execution_id = ? LIMIT 1", instanceID, executionID)
	var state core.WorkflowInstanceState
	if err := row.Scan(&state); err != nil {
		if err == sql.ErrNoRows {
			return backend.ErrInstanceNotFound
		}
	}

	if state == core.WorkflowInstanceStateActive {
		return backend.ErrInstanceNotFinished
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM `instances` WHERE id = ? AND execution_id = ?", instanceID, executionID); err != nil {
		if err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM `history` WHERE instance_id = ? AND execution_id = ?", instanceID, executionID); err != nil {
		if err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}
		return nil
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM `attributes` WHERE instance_id = ? AND execution_id = ?", instanceID, executionID); err != nil {
		if err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}
		return nil
	}

	return nil
}

func (sb *sqliteBackend) RemoveWorkflowInstances(ctx context.Context, options ...backend.RemovalOption) error {
	ro := backend.DefaultRemovalOptions
	for _, opt := range options {
		opt(&ro)
	}

	rows, err := sb.db.QueryContext(ctx, `SELECT id, execution_id FROM instances WHERE completed_at < ?`, ro.FinishedBefore)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}

	instanceIDs := []string{}
	executionIDs := []string{}
	for rows.Next() {
		var id, executionID string
		if err := rows.Scan(&id, &executionID); err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}

		instanceIDs = append(instanceIDs, id)
		executionIDs = append(executionIDs, executionID)
	}

	if rows.Err() != nil {
		return rows.Err()
	}

	batchSize := ro.BatchSize
	for i := 0; i < len(instanceIDs); i += batchSize {
		instanceIDs := instanceIDs[i:min(i+batchSize, len(instanceIDs))]
		executionIDs := executionIDs[i:min(i+batchSize, len(executionIDs))]

		tx, err := sb.db.BeginTx(ctx, nil)
		if err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}

		defer cleanup.Rollback(ctx, tx)

		placeholders := strings.Repeat(",?", len(instanceIDs)-1)
		whereCondition := fmt.Sprintf("id IN (?%v) AND execution_id IN (?%v)", placeholders, placeholders)
		args := make([]interface{}, 0, len(instanceIDs)*2)
		for i := range instanceIDs {
			args = append(args, instanceIDs[i])
		}
		for i := range executionIDs {
			args = append(args, executionIDs[i])
		}

		// Delete from instances, history and attributes tables
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM `instances` WHERE %v", whereCondition), args...); err != nil {
			if err != nil {
				return ctxerrors.Wrap(err, "execute sqlite")
			}
			return nil
		}

		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM `history` WHERE %v", whereCondition), args...); err != nil {
			if err != nil {
				return ctxerrors.Wrap(err, "execute sqlite")
			}
			return nil
		}

		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM `attributes` WHERE %v", whereCondition), args...); err != nil {
			if err != nil {
				return ctxerrors.Wrap(err, "execute sqlite")
			}
			return nil
		}

		if err := tx.Commit(); err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}
	}

	return nil
}

func (sb *sqliteBackend) CancelWorkflowInstance(ctx context.Context, instance *workflow.Instance, event *history.Event) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	instanceID := instance.InstanceID
	executionID := instance.ExecutionID

	// TODO: Combine with event insertion
	res := tx.QueryRowContext(ctx, "SELECT 1 FROM `instances` WHERE id = ? AND execution_id = ? LIMIT 1", instanceID, executionID)
	if err := res.Scan(new(int)); err != nil {
		if err == sql.ErrNoRows {
			return backend.ErrInstanceNotFound
		}

		if err != nil {
			return ctxerrors.Wrap(err, "execute sqlite")
		}

		return nil
	}

	if err := insertPendingEvents(ctx, tx, instance, []*history.Event{event}); err != nil {
		return ctxerrors.Wrap(err, "inserting cancellation event")
	}

	return tx.Commit()
}

func (sb *sqliteBackend) GetWorkflowInstanceHistory(ctx context.Context, instance *workflow.Instance, lastSequenceID *int64) ([]*history.Event, error) {
	tx, err := sb.db.BeginTx(ctx, &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		return nil, ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	h, err := getHistory(ctx, tx, instance, lastSequenceID)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "getting workflow history")
	}

	return h, nil
}

func (sb *sqliteBackend) GetWorkflowInstanceState(ctx context.Context, instance *workflow.Instance) (core.WorkflowInstanceState, error) {
	tx, err := sb.db.BeginTx(ctx, &sql.TxOptions{
		ReadOnly: true,
	})
	if err != nil {
		return core.WorkflowInstanceStateActive, ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	row := tx.QueryRowContext(
		ctx,
		"SELECT state FROM instances WHERE id = ? AND execution_id = ?",
		instance.InstanceID,
		instance.ExecutionID,
	)

	var state core.WorkflowInstanceState
	if err := row.Scan(&state); err != nil {
		if err == sql.ErrNoRows {
			return core.WorkflowInstanceStateActive, backend.ErrInstanceNotFound
		}
		return core.WorkflowInstanceStateActive, ctxerrors.Wrap(err, "scanning instance state")
	}

	return state, nil
}

func (sb *sqliteBackend) SignalWorkflow(ctx context.Context, instanceID string, event *history.Event) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	// TODO: Combine this with the event insertion
	var executionID string
	res := tx.QueryRowContext(ctx, "SELECT execution_id FROM `instances` WHERE id = ? AND state = ? LIMIT 1", instanceID, core.WorkflowInstanceStateActive)
	if err := res.Scan(&executionID); err == sql.ErrNoRows {
		return backend.ErrInstanceNotFound
	}

	if err := insertPendingEvents(ctx, tx, core.NewWorkflowInstance(instanceID, executionID), []*history.Event{event}); err != nil {
		return ctxerrors.Wrap(err, "inserting signal event")
	}

	return tx.Commit()
}

func (sb *sqliteBackend) PrepareWorkflowQueues(ctx context.Context, queues []workflow.Queue) error {
	return nil
}

func (sb *sqliteBackend) PrepareActivityQueues(ctx context.Context, queues []workflow.Queue) error {
	return nil
}

func (sb *sqliteBackend) GetWorkflowTask(ctx context.Context, queues []workflow.Queue) (*backend.WorkflowTask, error) {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	// Lock next workflow task by finding an unlocked instance with new events to process
	// (work around missing LIMIT support in sqlite driver for UPDATE statements by using sub-query)
	now := time.Now()

	args := []any{
		now.Add(sb.options.WorkflowLockTimeout), // new locked_until
		sb.workerName,
		now,                              // locked_until
		now,                              // sticky_until
		sb.workerName,                    // worker
		core.WorkflowInstanceStateActive, // state
	}

	for _, q := range queues {
		args = append(args, string(q))
	}

	args = append(args,
		now, // pending_event.visible_at
	)

	row := tx.QueryRowContext(
		ctx,
		fmt.Sprintf(`UPDATE instances
			SET locked_until = ?, worker = ?
			WHERE rowid = (
				SELECT rowid FROM instances i
					WHERE
						(i.locked_until IS NULL OR i.locked_until < ?)
						AND (i.sticky_until IS NULL OR i.sticky_until < ? OR i.worker = ?)
						AND i.state = ?
						AND i.completed_at IS NULL
						AND i.queue IN (?%s)
						AND EXISTS (
							SELECT 1
								FROM pending_events
								WHERE instance_id = i.id AND execution_id = i.execution_id AND (visible_at IS NULL OR visible_at <= ?)
						)
					LIMIT 1
			) RETURNING queue, id, execution_id, parent_instance_id, parent_execution_id, parent_schedule_event_id, metadata, sticky_until`, strings.Repeat(",?", len(queues)-1)),
		args...,
	)

	var queue, instanceID, executionID string
	var parentInstanceID, parentExecutionID *string
	var parentEventID *int64
	var metadataJson sql.NullString
	var stickyUntil *time.Time
	if err := row.Scan(&queue, &instanceID, &executionID, &parentInstanceID, &parentExecutionID, &parentEventID, &metadataJson, &stickyUntil); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}

		return nil, ctxerrors.Wrap(err, "locking workflow task")
	}

	var wfi *workflow.Instance
	if parentInstanceID != nil {
		wfi = core.NewSubWorkflowInstance(instanceID, executionID, core.NewWorkflowInstance(*parentInstanceID, *parentExecutionID), *parentEventID)
	} else {
		wfi = core.NewWorkflowInstance(instanceID, executionID)
	}

	var metadata *metadata.WorkflowMetadata
	if metadataJson.Valid {
		if err := json.Unmarshal([]byte(metadataJson.String), &metadata); err != nil {
			return nil, ctxerrors.Wrap(err, "parsing workflow metadata")
		}
	}

	t := &backend.WorkflowTask{
		ID:                    wfi.InstanceID,
		Queue:                 workflow.Queue(queue),
		WorkflowInstance:      wfi,
		WorkflowInstanceState: core.WorkflowInstanceStateActive,
		Metadata:              metadata,
		NewEvents:             []*history.Event{},
	}

	// Get new events
	pendingEvents, err := getPendingEvents(ctx, tx, wfi)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "getting pending events")
	}

	// Return if there aren't any new events
	if len(pendingEvents) == 0 {
		return nil, nil
	}

	t.NewEvents = pendingEvents

	// Get only most recent sequence ID
	// TODO: Denormalize to instances table
	row = tx.QueryRowContext(ctx, "SELECT sequence_id FROM `history` WHERE instance_id = ? AND execution_id = ? ORDER BY rowid DESC LIMIT 1", instanceID, executionID)
	if err := row.Scan(&t.LastSequenceID); err != nil {
		if err != sql.ErrNoRows {
			return nil, ctxerrors.Wrap(err, "getting most recent sequence id")
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, ctxerrors.Wrap(err, "execute sqlite")
	}

	return t, nil
}

// MARK: CompleteWorkflowTask
func (sb *sqliteBackend) CompleteWorkflowTask(
	ctx context.Context,
	task *backend.WorkflowTask,
	state core.WorkflowInstanceState,
	executedEvents, activityEvents, timerEvents []*history.Event,
	workflowEvents []*history.WorkflowEvent,
) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	instance := task.WorkflowInstance

	var completedAt *time.Time
	if state == core.WorkflowInstanceStateContinuedAsNew || state == core.WorkflowInstanceStateFinished {
		t := time.Now()
		completedAt = &t
	}

	// Unlock instance, but keep it sticky to the current worker
	if res, err := tx.ExecContext(
		ctx,
		`UPDATE instances SET locked_until = NULL, sticky_until = ?, completed_at = ?, state = ? WHERE id = ? AND execution_id = ? AND worker = ?`,
		time.Now().Add(sb.options.StickyTimeout),
		completedAt,
		state,
		instance.InstanceID,
		instance.ExecutionID,
		sb.workerName,
	); err != nil {
		return ctxerrors.Wrap(err, "unlocking workflow instance")
	} else if n, err := res.RowsAffected(); err != nil {
		return ctxerrors.Wrap(err, "checking for unlocked workflow instances")
	} else if n != 1 {
		return ctxerrors.New("could not find workflow instance to unlock")
	}

	// Remove handled events from task
	if len(executedEvents) > 0 {
		args := make([]interface{}, 0, len(executedEvents)+1)
		args = append(args, instance.InstanceID, instance.ExecutionID)
		for _, e := range executedEvents {
			args = append(args, e.ID)
		}

		// Remove from pending
		if _, err := tx.ExecContext(
			ctx,
			fmt.Sprintf(`DELETE FROM pending_events WHERE instance_id = ? AND execution_id = ? AND id IN (?%v)`, strings.Repeat(",?", len(executedEvents)-1)),
			args...,
		); err != nil {
			return ctxerrors.Wrap(err, "deleting handled new events")
		}
	}

	if err := insertEvents(ctx, tx, "history", instance, executedEvents); err != nil {
		return ctxerrors.Wrap(err, "inserting history events")
	}

	// Schedule activities
	for _, e := range activityEvents {
		a := e.Attributes.(*history.ActivityScheduledAttributes)
		queue := a.Queue
		if queue == "" {
			// Default to workflow queue
			queue = task.Queue
		}

		if err := scheduleActivity(ctx, tx, queue, instance, e); err != nil {
			return ctxerrors.Wrap(err, "scheduling activity")
		}
	}

	// Timer events
	if err := insertPendingEvents(ctx, tx, instance, timerEvents); err != nil {
		return ctxerrors.Wrap(err, "scheduling timers")
	}

	for _, event := range executedEvents {
		switch event.Type {
		case history.EventType_TimerCanceled:
			if err := removeFutureEvent(ctx, tx, instance, event.ScheduleEventID); err != nil {
				return ctxerrors.Wrap(err, "removing future event")
			}
		}
	}

	// Insert new workflow events
	groupedEvents := history.EventsByWorkflowInstance(workflowEvents)

	for targetInstance, events := range groupedEvents {
		// Are we creating a new sub-workflow instance?
		m := events[0]
		if m.HistoryEvent.Type == history.EventType_WorkflowExecutionStarted {
			a := m.HistoryEvent.Attributes.(*history.ExecutionStartedAttributes)

			queue := a.Queue
			if queue == "" {
				queue = task.Queue
			}

			// Create new instance
			if err := createInstance(ctx, tx, queue, m.WorkflowInstance, a.Metadata); err != nil {
				if errors.Is(err, backend.ErrInstanceAlreadyExists) {
					if err := insertPendingEvents(ctx, tx, instance, []*history.Event{
						history.NewPendingEvent(time.Now(), history.EventType_SubWorkflowFailed, &history.SubWorkflowFailedAttributes{
							Error: workflowerrors.FromError(backend.ErrInstanceAlreadyExists),
						}, history.ScheduleEventID(m.WorkflowInstance.ParentEventID)),
					}); err != nil {
						return ctxerrors.Wrap(err, "inserting sub-workflow failed event")
					}

					continue
				}

				return ctxerrors.Wrap(err, "creating sub-workflow instance")
			}
		}

		// Insert pending events for target instance
		historyEvents := []*history.Event{}
		for _, m := range events {
			historyEvents = append(historyEvents, m.HistoryEvent)
		}
		if err := insertPendingEvents(ctx, tx, &targetInstance, historyEvents); err != nil {
			return ctxerrors.Wrap(err, "inserting messages")
		}
	}

	if sb.options.RemoveContinuedAsNewInstances && state == core.WorkflowInstanceStateContinuedAsNew {
		if err := sb.removeWorkflowInstance(ctx, instance, tx); err != nil {
			return ctxerrors.Wrap(err, "removing old instance")
		}
	}

	return tx.Commit()
}

func (sb *sqliteBackend) ExtendWorkflowTask(ctx context.Context, task *backend.WorkflowTask) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	until := time.Now().Add(sb.options.WorkflowLockTimeout)
	res, err := tx.ExecContext(
		ctx,
		`UPDATE instances SET locked_until = ? WHERE id = ? AND execution_id = ? AND worker = ?`,
		until,
		task.WorkflowInstance.InstanceID,
		task.WorkflowInstance.ExecutionID,
		sb.workerName,
	)
	if err != nil {
		return ctxerrors.Wrap(err, "extending workflow task lock")
	}

	if rowsAffected, err := res.RowsAffected(); err != nil {
		return ctxerrors.Wrap(err, "determining if workflow task was extended")
	} else if rowsAffected == 0 {
		return ctxerrors.New("could not extend workflow task")
	}

	return tx.Commit()
}

func (sb *sqliteBackend) GetActivityTask(ctx context.Context, queues []workflow.Queue) (*backend.ActivityTask, error) {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	// Lock next activity
	// (work around missing LIMIT support in sqlite driver for UPDATE statements by using sub-query)
	now := time.Now()

	args := []interface{}{
		now.Add(sb.options.ActivityLockTimeout),
		sb.workerName,
		now,
	}

	for _, q := range queues {
		args = append(args, string(q))
	}

	row := tx.QueryRowContext(
		ctx,
		fmt.Sprintf(`UPDATE activities
			SET locked_until = ?, worker = ?
			WHERE rowid = (
				SELECT rowid FROM activities WHERE (locked_until IS NULL OR locked_until < ?) AND queue IN (?%s) LIMIT 1
			) RETURNING id, instance_id, execution_id, event_type, timestamp, schedule_event_id, visible_at, queue`, strings.Repeat(",?", len(queues)-1)),
		args...,
	)

	var instanceID, executionID string
	var activityQueue workflow.Queue
	event := &history.Event{}

	if err := row.Scan(
		&event.ID,
		&instanceID,
		&executionID,
		&event.Type,
		&event.Timestamp,
		&event.ScheduleEventID,
		&event.VisibleAt,
		&activityQueue,
	); err != nil {
		if err == sql.ErrNoRows {
			// No rows locked, just return
			return nil, nil
		}

		return nil, ctxerrors.Wrap(err, "scanning event")
	}

	var attributes []byte
	if err := tx.QueryRowContext(
		ctx, "SELECT data FROM attributes WHERE instance_id = ? AND execution_id = ? AND id = ?", instanceID, executionID, event.ID,
	).Scan(&attributes); err != nil {
		return nil, ctxerrors.Wrap(err, "scanning attributes")
	}

	a, err := history.DeserializeAttributes(event.Type, attributes)
	if err != nil {
		return nil, ctxerrors.Wrap(err, "deserializing attributes")
	}

	event.Attributes = a

	var metadataJson sql.NullString
	if err := tx.QueryRowContext(ctx, "SELECT metadata FROM instances WHERE id = ?", instanceID).Scan(&metadataJson); err != nil {
		return nil, ctxerrors.Wrap(err, "scanning metadata")
	}

	var metadata *workflow.Metadata
	if err := json.Unmarshal([]byte(metadataJson.String), &metadata); err != nil {
		return nil, ctxerrors.Wrap(err, "unmarshaling metadata")
	}

	t := &backend.ActivityTask{
		ID:               event.ID,
		ActivityID:       event.ID,
		Queue:            activityQueue,
		WorkflowInstance: core.NewWorkflowInstance(instanceID, executionID),
		Event:            event,
	}

	if err := tx.Commit(); err != nil {
		return nil, ctxerrors.Wrap(err, "execute sqlite")
	}

	return t, nil
}

func (sb *sqliteBackend) CompleteActivityTask(ctx context.Context, task *backend.ActivityTask, result *history.Event) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	// Remove activity but keep the attributes, they are still needed for the history
	if res, err := tx.ExecContext(
		ctx,
		`DELETE FROM activities WHERE instance_id = ? AND execution_id = ? AND id = ? AND worker = ?`,
		task.WorkflowInstance.InstanceID,
		task.WorkflowInstance.ExecutionID,
		task.ActivityID,
		sb.workerName,
	); err != nil {
		return ctxerrors.Wrap(err, "unlocking instance")
	} else if n, err := res.RowsAffected(); err != nil {
		return ctxerrors.Wrap(err, "checking for deleted activities")
	} else if n != 1 {
		return ctxerrors.New("could not find activity to delete")
	}

	// Insert new event generated during this workflow execution
	if err := insertPendingEvents(ctx, tx, task.WorkflowInstance, []*history.Event{result}); err != nil {
		return ctxerrors.Wrap(err, "inserting new events for completed activity")
	}

	return tx.Commit()
}

func (sb *sqliteBackend) ExtendActivityTask(ctx context.Context, task *backend.ActivityTask) error {
	tx, err := sb.db.BeginTx(ctx, nil)
	if err != nil {
		return ctxerrors.Wrap(err, "execute sqlite")
	}
	defer cleanup.Rollback(ctx, tx)

	until := time.Now().Add(sb.options.ActivityLockTimeout)
	res, err := tx.ExecContext(
		ctx,
		`UPDATE activities SET locked_until = ? WHERE id = ? AND worker = ?`,
		until,
		task.ActivityID,
		sb.workerName,
	)
	if err != nil {
		return ctxerrors.Wrap(err, "extending activity lock")
	}

	if rowsAffected, err := res.RowsAffected(); err != nil {
		return ctxerrors.Wrap(err, "determining if activity was extended")
	} else if rowsAffected == 0 {
		return ctxerrors.New("could not extend activity")
	}

	return tx.Commit()
}

// getWorkerName returns the worker name from options, or generates a UUID-based name if not set.
func getWorkerName(options *options) string {
	if options.WorkerName != "" {
		return options.WorkerName
	}
	return fmt.Sprintf("worker-%v", uuid.NewString())
}
