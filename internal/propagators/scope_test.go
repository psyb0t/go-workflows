package propagators

import (
	"context"
	"testing"

	"github.com/psyb0t/ctxscope"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/internal/sync"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestScopeRoundTrip(t *testing.T) {
	t.Parallel()
	prop := &ScopeContextPropagator{}
	ctx := ctxscope.Set(context.Background(), ctxscope.Attr("request_id", "test-request"))
	metadata := workflow.Metadata{}
	require.NoError(t, prop.Inject(ctx, &metadata))
	wf, err := prop.ExtractToWorkflow(sync.Background(), &metadata)
	require.NoError(t, err)
	forwarded := workflow.Metadata{}
	require.NoError(t, prop.InjectFromWorkflow(wf, &forwarded))
	restored, err := prop.Extract(context.Background(), &forwarded)
	require.NoError(t, err)
	before, err := ctxscope.ToJSON(ctx)
	require.NoError(t, err)
	after, err := ctxscope.ToJSON(restored)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

func TestScopeMetadataBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data string
		invalid    bool
	}{
		{name: "legacy empty"},
		{name: "empty scope", data: "{}"},
		{name: "malformed", data: "{", invalid: true},
		{name: "array", data: "[]", invalid: true},
		{name: "null", data: "null", invalid: true},
		{name: "nested", data: `{"nested":{}}`, invalid: true},
		{name: "list", data: `{"nested":[]}`, invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prop := &ScopeContextPropagator{}
			metadata := workflow.Metadata{scopeMetadataKey: tc.data}
			_, ordinaryErr := prop.Extract(context.Background(), &metadata)
			_, workflowErr := prop.ExtractToWorkflow(sync.Background(), &metadata)
			if tc.invalid {
				require.Error(t, ordinaryErr)
				require.Error(t, workflowErr)
				return
			}
			require.NoError(t, ordinaryErr)
			require.NoError(t, workflowErr)
		})
	}
}
