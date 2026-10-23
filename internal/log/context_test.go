package log

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/psyb0t/ctxscope"
	"github.com/stretchr/testify/require"
)

func TestContextLoggerPreservesSinkAndScope(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&output, nil))
	scoped := ScopeLogger(base)
	require.Same(t, scoped, ScopeLogger(scoped))
	ctx := ctxscope.Set(context.Background(), ctxscope.Attr("request_id", "test-request"))
	WithContext(scoped, ctx).With("operation", "test").Info("completed")
	var record map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "test-request", record["request_id"])
	require.Equal(t, "test", record["operation"])
	require.Equal(t, "INFO", record["level"])
	require.Equal(t, "completed", record["msg"])
	require.Equal(t, 1, bytes.Count(output.Bytes(), []byte(`"request_id"`)))
	output.Reset()
	WithContext(scoped, ctx).WithGroup("details").Info("grouped", "value", "ok")
	require.NoError(t, json.Unmarshal(output.Bytes(), &record))
	require.Equal(t, "ok", record["details"].(map[string]any)["value"])
}
