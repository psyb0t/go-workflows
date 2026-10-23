package workflow

import (
	"log/slog"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/psyb0t/go-workflows/backend/converter"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/contextvalue"
	"github.com/psyb0t/go-workflows/internal/sync"
	"github.com/psyb0t/go-workflows/internal/workflowstate"
)

func Test_Timer_Cancellation(t *testing.T) {
	state := workflowstate.NewWorkflowState(
		core.NewWorkflowInstance("a", ""), slog.Default(), noop.NewTracerProvider().Tracer("test"), clock.New())

	ctx, cancel := WithCancel(sync.Background())
	ctx = contextvalue.WithConverter(ctx, converter.DefaultConverter)
	ctx = workflowstate.WithWorkflowState(ctx, state)

	c := sync.NewCoroutine(ctx, func(ctx Context) error {
		f := ScheduleTimer(ctx, time.Second*1)
		_, err := f.Get(ctx)
		require.NoError(t, err)

		// Block workflow
		_, err = sync.NewFuture[int]().Get(ctx)
		require.NoError(t, err)

		return nil
	})
	c.Execute()
	require.False(t, c.Finished())

	// Fire timer
	cmd := state.CommandByScheduleEventID(1)
	cmd.Commit()
	cmd.Done()
	fs, ok := state.FutureByScheduleEventID(1)
	require.True(t, ok)
	if err := fs.Set(nil, nil); err != nil {
		t.Errorf("unexpected operation error: %v", err)
	}

	c.Execute()
	require.False(t, c.Finished())

	cancel()
}
