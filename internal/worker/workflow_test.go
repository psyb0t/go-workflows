package worker

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/metadata"
	"github.com/psyb0t/go-workflows/backend/mocks"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/fn"
	im "github.com/psyb0t/go-workflows/internal/metrics"
	"github.com/psyb0t/go-workflows/registry"
	"github.com/psyb0t/go-workflows/workflow"
	"github.com/psyb0t/go-workflows/workflow/executor"
)

// closedExecutorCache always returns an executor the cache already closed,
// as happens when an entry expires between lookup and task start.
type closedExecutorCache struct {
	closed executor.WorkflowExecutor
	stored []executor.WorkflowExecutor
}

func (c *closedExecutorCache) Get(
	ctx context.Context,
	instance *core.WorkflowInstance,
) (executor.WorkflowExecutor, bool, error) {
	return c.closed, true, nil
}

func (c *closedExecutorCache) Store(
	ctx context.Context,
	instance *core.WorkflowInstance,
	e executor.WorkflowExecutor,
) error {
	c.stored = append(c.stored, e)

	return nil
}

func (c *closedExecutorCache) Evict(ctx context.Context, instance *core.WorkflowInstance) error {
	return nil
}

func (c *closedExecutorCache) StartEviction(ctx context.Context) {}

func TestWorkflowTaskWorkerReplacesClosedCachedExecutor(t *testing.T) {
	t.Parallel()

	r := registry.New()
	finish := func(ctx workflow.Context) error {
		return nil
	}
	require.NoError(t, r.RegisterWorkflow(finish))

	options := backend.ApplyOptions()
	tracer := noop.NewTracerProvider().Tracer(backend.TracerName)
	mockBackend := &mocks.MockBackend{}
	mockBackend.On("Options").Return(options)
	mockBackend.On("Metrics").Return(im.NewNoopMetricsClient())
	mockBackend.On("Tracer").Return(tracer)

	instance := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
	closed, err := executor.NewExecutor(
		slog.Default(),
		tracer,
		r,
		options.Converter,
		options.ContextPropagators,
		mockBackend,
		instance,
		&metadata.WorkflowMetadata{},
		clock.New(),
		options.MaxHistorySize,
	)
	require.NoError(t, err)
	closed.Close()

	cache := &closedExecutorCache{closed: closed}
	taskWorker := &WorkflowTaskWorker{
		backend:  mockBackend,
		registry: r,
		cache:    cache,
		logger:   slog.Default(),
	}

	task := &backend.WorkflowTask{
		ID:               uuid.NewString(),
		WorkflowInstance: instance,
		Metadata:         &metadata.WorkflowMetadata{},
		NewEvents: []*history.Event{
			history.NewPendingEvent(
				time.Now(),
				history.EventType_WorkflowExecutionStarted,
				&history.ExecutionStartedAttributes{Name: fn.Name(finish)},
			),
		},
	}

	result, err := taskWorker.Execute(t.Context(), task)
	require.NoError(t, err)
	require.Equal(t, core.WorkflowInstanceStateFinished, result.State)

	require.Len(t, cache.stored, 2)
	require.Same(t, closed, cache.stored[0])
	require.NotSame(t, closed, cache.stored[1])

	cache.stored[1].Close()
}
