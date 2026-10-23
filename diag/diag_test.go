package diag_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/sqlite"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/diag"
	"github.com/psyb0t/go-workflows/worker"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestDiagnosticsWithRealStorage(t *testing.T) {
	t.Parallel()
	b := sqlite.NewInMemoryBackend()
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	c := client.New(b)
	definition := func(workflow.Context) error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	w := worker.New(b, nil)
	require.NoError(t, w.RegisterWorkflow(definition))
	require.NoError(t, w.Start(ctx))
	t.Cleanup(func() {
		cancel()
		require.NoError(t, w.WaitForCompletion())
	})
	instances := make([]*workflow.Instance, 0, 3)
	for _, id := range []string{"first", "second", "third"} {
		instance, err := c.CreateWorkflowInstance(context.Background(), client.WorkflowInstanceOptions{InstanceID: id}, definition)
		require.NoError(t, err)
		_, err = client.GetWorkflowResult[any](ctx, c, instance, 5*time.Second)
		require.NoError(t, err)
		instances = append(instances, instance)
	}
	mux := diag.NewServeMux(b)
	request := func(method, path string) *httptest.ResponseRecorder {
		t.Helper()
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		return response
	}
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{name: "index", method: http.MethodGet, path: "/", status: http.StatusOK},
		{name: "stats", method: http.MethodGet, path: "/api/stats", status: http.StatusOK},
		{name: "list", method: http.MethodGet, path: "/api/", status: http.StatusOK},
		{name: "malformed count", method: http.MethodGet, path: "/api/?count=invalid", status: http.StatusBadRequest},
		{name: "write rejected", method: http.MethodPost, path: "/api/", status: http.StatusMethodNotAllowed},
		{name: "missing instance", method: http.MethodGet, path: "/api/missing/missing", status: http.StatusNotFound},
		{name: "missing tree", method: http.MethodGet, path: "/api/missing/missing/tree", status: http.StatusNotFound},
		{name: "unknown operation", method: http.MethodGet, path: "/api/missing/missing/other", status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			response := request(tc.method, tc.path)
			require.Equal(t, tc.status, response.Code)
		})
	}
	index := request(http.MethodGet, "/")
	require.Contains(t, index.Body.String(), `type="module"`)
	for _, field := range strings.Split(index.Body.String(), `"`) {
		if !strings.HasPrefix(field, "./assets/") {
			continue
		}
		asset := request(http.MethodGet, strings.TrimPrefix(field, "."))
		require.Equal(t, http.StatusOK, asset.Code)
		require.NotEmpty(t, asset.Body.Bytes())
	}
	seen := map[string]bool{}
	cursor := ""
	for range len(instances) {
		response := request(http.MethodGet, "/api/?count=1&after="+cursor)
		require.Equal(t, http.StatusOK, response.Code)
		var page []*diag.WorkflowInstanceRef
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &page))
		require.Len(t, page, 1)
		instance := page[0].Instance
		require.False(t, seen[instance.InstanceID])
		seen[instance.InstanceID] = true
		cursor = instance.InstanceID + ":" + instance.ExecutionID
		path := "/api/" + instance.InstanceID + "/" + instance.ExecutionID
		details := request(http.MethodGet, path)
		require.Equal(t, http.StatusOK, details.Code)
		var info diag.WorkflowInstanceInfo
		require.NoError(t, json.Unmarshal(details.Body.Bytes(), &info))
		require.Equal(t, instance.InstanceID, info.Instance.InstanceID)
		require.NotEmpty(t, info.History)
		tree := request(http.MethodGet, path+"/tree")
		require.Equal(t, http.StatusOK, tree.Code)
		var node diag.WorkflowInstanceTree
		require.NoError(t, json.Unmarshal(tree.Body.Bytes(), &node))
		require.Equal(t, instance.InstanceID, node.Instance.InstanceID)
	}
	response := request(http.MethodGet, "/api/?count=1&after="+cursor)
	var last []*diag.WorkflowInstanceRef
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &last))
	require.Empty(t, last)
	require.Len(t, seen, len(instances))
}

func TestDiagnosticsStorageFailure(t *testing.T) {
	t.Parallel()
	b := sqlite.NewInMemoryBackend()
	require.NoError(t, b.Close())
	mux := diag.NewServeMux(b)
	for _, tc := range []struct {
		path   string
		status int
	}{
		{path: "/api/stats", status: http.StatusInternalServerError},
		{path: "/api/", status: http.StatusInternalServerError},
		{path: "/api/instance/execution", status: http.StatusNotFound},
		{path: "/api/instance/execution/tree", status: http.StatusNotFound},
	} {
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()
			response := httptest.NewRecorder()
			require.NotPanics(t, func() {
				mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			})
			require.Equal(t, tc.status, response.Code)
		})
	}
}
