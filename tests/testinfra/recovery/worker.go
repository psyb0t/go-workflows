package recovery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxerrors/commerr"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/postgres"
	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/registry"
	"github.com/psyb0t/go-workflows/tests/testinfra"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

const (
	RecoverySQLite       testinfra.Kind = "sqlite"
	RecoveryActivity                    = "recovery-record"
	RecoverySignal                      = "resume"
	RecoveryLease                       = 1500 * time.Millisecond
	RecoveryPoll                        = 20 * time.Millisecond
	recoveryHeartbeat                   = 100 * time.Millisecond
	recoveryHTTPTimeout                 = 20 * time.Second
	recoveryInputLimit                  = 16 * 1024
	RecoveryRoleBoth                    = "both"
	RecoveryRoleWorkflow                = "workflow"
	RecoveryRoleActivity                = "activity"
)

type RecoveryConfig struct {
	Kind          testinfra.Kind
	Host          string
	Port          int
	User          string
	Password      string
	Database      string
	SQLitePath    string
	Name          string
	CallbackURL   string
	WorkflowQueue workflow.Queue
	ActivityQueue workflow.Queue
	Role          string
}

// OpenRecoveryBackend uses the production constructors and optional migrations.
func OpenRecoveryBackend(c RecoveryConfig, migrate bool) (backend.Backend, error) {
	opts := []backend.BackendOption{
		backend.WithWorkerName(c.Name),
		backend.WithStickyTimeout(0),
		backend.WithWorkflowLockTimeout(RecoveryLease),
		backend.WithActivityLockTimeout(RecoveryLease),
	}
	switch c.Kind {
	case testinfra.Postgres:
		return postgres.NewPostgresBackend(c.Host, c.Port, c.User, c.Password, c.Database,
			postgres.WithApplyMigrations(migrate), postgres.WithBackendOptions(opts...)), nil
	case RecoverySQLite:
		if c.SQLitePath == "" {
			return nil, ctxerrors.Wrap(commerr.ErrInvalidArgument, "SQLite path is required")
		}
		return sqlite.NewSqliteBackend(c.SQLitePath, sqlite.WithApplyMigrations(migrate), sqlite.WithBackendOptions(opts...)), nil
	default:
		return nil, ctxerrors.Wrap(commerr.ErrInvalidArgument, "unsupported recovery backend")
	}
}

// RunRecoveryWorker runs the same registered worker in an independently killable process.
func RunRecoveryWorker(ctx context.Context, input io.Reader, ready io.Writer) (resultErr error) {
	data, err := io.ReadAll(io.LimitReader(input, recoveryInputLimit+1))
	if err != nil {
		return ctxerrors.Wrap(err, "read worker configuration")
	}
	if len(data) > recoveryInputLimit {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "worker configuration exceeds size limit")
	}
	var c RecoveryConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&c); err != nil {
		return ctxerrors.Wrap(err, "decode worker configuration")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "worker configuration must contain one JSON value")
	}
	if c.Role != RecoveryRoleBoth && c.Role != RecoveryRoleWorkflow && c.Role != RecoveryRoleActivity {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "invalid worker role")
	}
	if c.Name == "" || c.CallbackURL == "" {
		return ctxerrors.Wrap(commerr.ErrInvalidArgument, "worker name and callback are required")
	}
	b, err := OpenRecoveryBackend(c, false)
	if err != nil {
		return ctxerrors.Wrap(err, "open worker backend")
	}
	defer func() {
		if err := b.Close(); err != nil {
			resultErr = errors.Join(resultErr, ctxerrors.Wrap(err, "close worker backend"))
		}
	}()
	opts := worker.DefaultOptions
	opts.WorkflowPollers, opts.ActivityPollers = 1, 1
	opts.WorkflowPollingInterval, opts.ActivityPollingInterval = RecoveryPoll, RecoveryPoll
	opts.WorkflowHeartbeatInterval, opts.ActivityHeartbeatInterval = recoveryHeartbeat, recoveryHeartbeat
	opts.MaxParallelWorkflowTasks, opts.MaxParallelActivityTasks = 4, 4
	opts.WorkflowQueues = []workflow.Queue{c.WorkflowQueue}
	opts.ActivityQueues = []workflow.Queue{c.ActivityQueue}
	w := worker.New(b, &opts)
	if c.Role == RecoveryRoleWorkflow {
		w = worker.NewWorkflowWorker(b, &opts.WorkflowWorkerOptions)
	}
	if c.Role == RecoveryRoleActivity {
		w = worker.NewActivityWorker(b, &opts.ActivityWorkerOptions)
	}
	if err := w.RegisterWorkflow(RecoveryWorkflow); err != nil {
		return ctxerrors.Wrap(err, "register recovery workflow")
	}
	activity := recoveryActivity{config: c}
	if err := w.RegisterActivity(activity.record, registry.WithName(RecoveryActivity)); err != nil {
		return ctxerrors.Wrap(err, "register recovery activity")
	}
	workerContext, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := w.Start(workerContext); err != nil {
		return ctxerrors.Wrap(err, "start recovery worker")
	}
	defer func() {
		cancel()
		if err := w.WaitForCompletion(); err != nil {
			resultErr = errors.Join(resultErr, ctxerrors.Wrap(err, "wait for recovery worker"))
		}
	}()
	if _, err := io.WriteString(ready, "ready\n"); err != nil {
		return ctxerrors.Wrap(err, "report worker readiness")
	}
	<-ctx.Done()
	return nil
}

type RecoveryInput struct {
	ID    string
	Queue workflow.Queue
	Wait  string
	Delay time.Duration
	Retry workflow.RetryOptions
}

type RecoveryResult struct {
	First  string
	Second string
	Signal string
}

func RecoveryWorkflow(ctx workflow.Context, input RecoveryInput) (RecoveryResult, error) {
	options := workflow.ActivityOptions{Queue: input.Queue, RetryOptions: input.Retry}
	first, err := workflow.ExecuteActivity[string](ctx, options, RecoveryActivity, input.ID, "first").Get(ctx)
	if err != nil {
		return RecoveryResult{}, ctxerrors.Wrap(err, "first recovery activity")
	}
	result := RecoveryResult{First: first}
	if input.Wait == "signal" {
		workflow.Select(ctx,
			workflow.Receive(workflow.NewSignalChannel[string](ctx, RecoverySignal), func(_ workflow.Context, value string, _ bool) {
				result.Signal = value
			}),
			workflow.Receive(ctx.Done(), func(workflow.Context, struct{}, bool) {}),
		)
		if err := ctx.Err(); err != nil {
			return result, ctxerrors.Wrap(err, "wait for recovery signal")
		}
	}
	if input.Wait == "timer" {
		if err := workflow.Sleep(ctx, input.Delay); err != nil {
			return result, ctxerrors.Wrap(err, "wait for recovery timer")
		}
	}
	second, err := workflow.ExecuteActivity[string](ctx, options, RecoveryActivity, input.ID, "second").Get(ctx)
	if err != nil {
		return result, ctxerrors.Wrap(err, "second recovery activity")
	}
	result.Second = second
	return result, nil
}

type RecoveryCall struct {
	ID     string
	Step   string
	Worker string
}

type RecoveryReply struct {
	Value   string
	Failure string
}

type recoveryActivity struct{ config RecoveryConfig }

func (a recoveryActivity) record(ctx context.Context, id, step string) (value string, resultErr error) {
	data, err := json.Marshal(RecoveryCall{ID: id, Step: step, Worker: a.config.Name})
	if err != nil {
		return "", ctxerrors.Wrap(err, "encode recovery call")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.CallbackURL, bytes.NewReader(data))
	if err != nil {
		return "", ctxerrors.Wrap(err, "create recovery callback")
	}
	response, err := (&http.Client{Timeout: recoveryHTTPTimeout}).Do(request)
	if err != nil {
		return "", ctxerrors.Wrap(err, "call recovery fixture")
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			resultErr = errors.Join(resultErr, ctxerrors.Wrap(err, "close recovery response"))
		}
	}()
	if response.StatusCode != http.StatusOK {
		return "", ctxerrors.Wrap(commerr.ErrFetchFailed, "recovery callback failed")
	}
	var reply RecoveryReply
	if err := json.NewDecoder(io.LimitReader(response.Body, recoveryInputLimit)).Decode(&reply); err != nil {
		return "", ctxerrors.Wrap(err, "decode recovery reply")
	}
	if reply.Failure == "permanent" {
		return "", workflow.NewPermanentError(ctxerrors.Wrap(commerr.ErrInvalidArgument, "permanent recovery failure"))
	}
	if reply.Failure != "" {
		return "", ctxerrors.Wrap(commerr.ErrFetchFailed, "transient recovery failure")
	}
	return reply.Value, nil
}
