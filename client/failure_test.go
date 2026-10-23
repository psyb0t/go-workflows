package client

import (
	"context"
	"testing"
	"time"

	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/backend/mocks"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/internal/metrics"
	"github.com/psyb0t/go-workflows/internal/workflowerrors"
	"github.com/psyb0t/go-workflows/workflow"
)

func failureBackend(t *testing.T) *mocks.MockBackend {
	t.Helper()
	b := mocks.NewMockBackend(t)
	b.EXPECT().Options().Return(backend.ApplyOptions()).Maybe()
	b.EXPECT().Tracer().Return(noop.NewTracerProvider().Tracer("client-test")).Maybe()
	b.EXPECT().Metrics().Return(metrics.NewNoopMetricsClient()).Maybe()
	return b
}

func TestClientBoundaryFailures(t *testing.T) {
	t.Parallel()
	t.Run("missing identity", func(t *testing.T) {
		t.Parallel()
		_, err := New(failureBackend(t)).CreateWorkflowInstance(context.Background(), WorkflowInstanceOptions{}, "definition")
		require.ErrorIs(t, err, commerr.ErrInvalidArgument)
	})
	t.Run("unserializable input", func(t *testing.T) {
		t.Parallel()
		_, err := New(failureBackend(t)).CreateWorkflowInstance(context.Background(), WorkflowInstanceOptions{InstanceID: "instance"}, "definition", make(chan int))
		require.Error(t, err)
	})
	t.Run("creation fails", func(t *testing.T) {
		t.Parallel()
		b := failureBackend(t)
		b.EXPECT().CreateWorkflowInstance(mock.Anything, mock.Anything, mock.Anything).Return(commerr.ErrFetchFailed).Once()
		_, err := New(b).CreateWorkflowInstance(context.Background(), WorkflowInstanceOptions{InstanceID: "instance"}, "definition")
		require.ErrorIs(t, err, commerr.ErrFetchFailed)
	})
	t.Run("unserializable signal", func(t *testing.T) {
		t.Parallel()
		require.Error(t, New(failureBackend(t)).SignalWorkflow(context.Background(), "instance", "signal", make(chan int)))
	})
	t.Run("signal fails", func(t *testing.T) {
		t.Parallel()
		b := failureBackend(t)
		b.EXPECT().SignalWorkflow(mock.Anything, "instance", mock.Anything).Return(commerr.ErrNotFound).Once()
		require.ErrorIs(t, New(b).SignalWorkflow(context.Background(), "instance", "signal", "value"), commerr.ErrNotFound)
	})
}

func TestClientResultFailures(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                          string
		kind                          history.EventType
		attributes                    any
		stateErr, historyErr, wantErr error
	}{
		{name: "state fails", stateErr: commerr.ErrFetchFailed, wantErr: commerr.ErrFetchFailed},
		{name: "history fails", historyErr: commerr.ErrFetchFailed, wantErr: commerr.ErrFetchFailed},
		{name: "missing result"},
		{name: "canceled", kind: history.EventType_WorkflowExecutionCanceled, wantErr: ErrWorkflowCanceled},
		{name: "terminated", kind: history.EventType_WorkflowExecutionTerminated, wantErr: ErrWorkflowTerminated},
		{name: "bad result", kind: history.EventType_WorkflowExecutionFinished, attributes: &history.ExecutionCompletedAttributes{Result: []byte("{")}},
		{name: "bad continued result", kind: history.EventType_WorkflowExecutionContinuedAsNew, attributes: &history.ExecutionContinuedAsNewAttributes{Result: []byte("{")}},
		{name: "workflow failure", kind: history.EventType_WorkflowExecutionFinished, attributes: &history.ExecutionCompletedAttributes{Error: workflowerrors.NewPermanentError(commerr.ErrFetchFailed)}},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := failureBackend(t)
			instance := core.NewWorkflowInstance("instance", "execution")
			b.EXPECT().GetWorkflowInstanceState(mock.Anything, instance).Return(core.WorkflowInstanceStateFinished, tc.stateErr).Once()
			if tc.stateErr == nil {
				events := []*history.Event{history.NewPendingEvent(time.Now(), history.EventType_WorkflowExecutionStarted, &history.ExecutionStartedAttributes{})}
				if tc.name != "missing result" {
					events = append(events, history.NewPendingEvent(time.Now(), tc.kind, tc.attributes))
				}
				b.EXPECT().GetWorkflowInstanceHistory(mock.Anything, instance, (*int64)(nil)).Return(events, tc.historyErr).Once()
			}
			value, err := GetWorkflowResult[int](context.Background(), New(b), instance, time.Second)
			require.Zero(t, value)
			require.Error(t, err)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			if tc.name == "workflow failure" {
				require.False(t, workflow.CanRetry(err))
			}
		})
	}
}

func TestAutomaticExpirationPolicy(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                 string
		supported            bool
		createErr, signalErr error
	}{
		{name: "unsupported"},
		{name: "create", supported: true},
		{name: "update existing", supported: true, createErr: backend.ErrInstanceAlreadyExists},
		{name: "create fails", supported: true, createErr: commerr.ErrFetchFailed},
		{name: "update fails", supported: true, createErr: backend.ErrInstanceAlreadyExists, signalErr: commerr.ErrFetchFailed},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := failureBackend(t)
			b.EXPECT().FeatureSupported(backend.Feature_Expiration).Return(tc.supported).Once()
			if tc.supported {
				b.EXPECT().CreateWorkflowInstance(mock.Anything, mock.Anything, mock.Anything).Return(tc.createErr).Once()
				if tc.createErr == backend.ErrInstanceAlreadyExists {
					b.EXPECT().SignalWorkflow(mock.Anything, expirationWorkflowInstanceID, mock.Anything).Return(tc.signalErr).Once()
				}
			}
			err := New(b).StartAutoExpiration(context.Background(), time.Hour)
			if !tc.supported {
				var unsupported backend.ErrNotSupported
				require.ErrorAs(t, err, &unsupported)
				return
			}
			if tc.signalErr != nil || tc.createErr == commerr.ErrFetchFailed {
				require.ErrorIs(t, err, commerr.ErrFetchFailed)
				return
			}
			require.NoError(t, err)
		})
	}
}
