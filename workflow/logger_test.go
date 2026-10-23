package workflow

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/contextvalue"
	workflowlog "github.com/psyb0t/go-workflows/internal/log"
	"github.com/psyb0t/go-workflows/internal/sync"
	"github.com/psyb0t/go-workflows/internal/workflowstate"
)

func TestScopedLoggerSuppressesReplay(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	logger := workflowlog.ScopeLogger(slog.New(slog.NewJSONHandler(&output, nil)))
	state := workflowstate.NewWorkflowState(core.NewWorkflowInstance("instance", "execution"), logger, noop.NewTracerProvider().Tracer("test"), clock.New())
	ctx := workflowstate.WithWorkflowState(sync.Background(), state)
	ctx = contextvalue.WithLogScope(ctx, []byte(`{"request_id":"request"}`))
	Logger(ctx).WithGroup("event").Info("live", "value", "ok")
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "live", record["msg"])
	before := output.String()
	state.SetReplaying(true)
	Logger(ctx).With("operation", "replay").Info("replayed")
	require.Equal(t, before, output.String())
	state.SetReplaying(false)
	output.Reset()
	Logger(ctx).Info("resumed")
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "request", record["request_id"])
	require.Equal(t, "instance", record["workflow_id"])
	require.Equal(t, "execution", record["workflow_run_id"])
}
