package tester

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestTesterInvalidRegistrations(t *testing.T) {
	t.Parallel()
	definition := func(workflow.Context) error { return nil }
	for _, invalid := range []any{nil, 42, func() {}, func(workflow.Context) {}} {
		t.Run(fmt.Sprintf("%T", invalid), func(t *testing.T) {
			t.Parallel()
			w := NewWorkflowTester[any](definition)
			require.Panics(t, func() { w.OnActivityByName("invalid", invalid) })
			require.Panics(t, func() { w.OnActivity(invalid) })
			require.Panics(t, func() { w.OnSubWorkflowByName("invalid", invalid) })
			require.Panics(t, func() { w.OnSubWorkflow(invalid) })
		})
	}
	w := NewWorkflowTester[any](definition)
	require.NotNil(t, w.ActivityMock())
	missing := core.NewWorkflowInstance("missing", "missing")
	require.ErrorIs(t, w.CancelWorkflowInstance(missing), backend.ErrInstanceNotFound)
	require.ErrorIs(t, w.SignalWorkflowInstance(missing, "signal", true), backend.ErrInstanceNotFound)
	require.Panics(t, func() { w.SignalWorkflow("signal", true) })
	require.Panics(t, func() { NewWorkflowTester[string](func(workflow.Context) (int, error) { return 0, nil }) })
	require.Panics(t, func() { NewWorkflowTester[any](func() error { return nil }) })
	inputs := NewWorkflowTester[any](func(workflow.Context, any) error { return nil })
	require.Panics(t, func() { inputs.Execute(context.Background(), make(chan int)) })
}
