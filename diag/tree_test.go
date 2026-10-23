package diag_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestDiagnosticSubworkflowTree(t *testing.T) {
	t.Parallel()
	b := sqlite.NewInMemoryBackend(sqlite.WithBackendOptions(backend.WithStickyTimeout(0)))
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	w := worker.New(b, nil)
	child := func(workflow.Context) error { return commerr.ErrInvalidState }
	childID := uuid.NewString()
	parent := func(ctx workflow.Context) error {
		_, err := workflow.CreateSubWorkflowInstance[any](ctx, workflow.SubWorkflowOptions{InstanceID: childID, RetryOptions: workflow.DefaultSubWorkflowRetryOptions}, child).Get(ctx)
		return err
	}
	require.NoError(t, w.RegisterWorkflow(child))
	require.NoError(t, w.RegisterWorkflow(parent))
	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() { cancel(); require.NoError(t, w.WaitForCompletion()) })
	c := client.New(b)
	instance, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{InstanceID: uuid.NewString()}, parent)
	require.NoError(t, err)
	_, err = client.GetWorkflowResult[any](ctx, c, instance, 5*time.Second)
	require.Error(t, err)
	tree, err := b.GetWorkflowTree(ctx, instance)
	require.NoError(t, err)
	require.True(t, tree.Error)
	require.Len(t, tree.Children, 1)
	require.Equal(t, childID, tree.Children[0].Instance.InstanceID)
	require.True(t, tree.Children[0].Error)
	require.NotEmpty(t, tree.Children[0].WorkflowName)
	fromChild, err := b.GetWorkflowTree(ctx, tree.Children[0].Instance)
	require.NoError(t, err)
	require.Equal(t, instance.InstanceID, fromChild.Instance.InstanceID)
	require.Len(t, fromChild.Children, 1)

	mux := diag.NewServeMux(b)
	path := "/api/" + instance.InstanceID + "/" + instance.ExecutionID
	for _, endpoint := range []string{"/api/", "/api/stats", path, path + "/tree"} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			response := &brokenResponse{header: http.Header{}}
			require.NotPanics(t, func() { mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, endpoint, nil)) })
			require.Equal(t, http.StatusInternalServerError, response.status)
			require.Equal(t, "application/json", response.header.Get("Content-Type"))
		})
	}
}

func TestDiagnosticTreeMissingParent(t *testing.T) {
	t.Parallel()
	b := sqlite.NewInMemoryBackend()
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	parent := core.NewWorkflowInstance(uuid.NewString(), uuid.NewString())
	child := core.NewSubWorkflowInstance(uuid.NewString(), uuid.NewString(), parent, 1)
	require.NoError(t, b.CreateWorkflowInstance(context.Background(), child, history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{Queue: workflow.QueueDefault})))
	tree, err := b.GetWorkflowTree(context.Background(), child)
	require.ErrorIs(t, err, commerr.ErrNotFound)
	require.Nil(t, tree)
}

type brokenResponse struct {
	header http.Header
	status int
}

func (w *brokenResponse) Header() http.Header       { return w.header }
func (w *brokenResponse) WriteHeader(status int)    { w.status = status }
func (w *brokenResponse) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
