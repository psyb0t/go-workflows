package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/core"
	"github.com/psyb0t/go-workflows/tests/testinfra"
	recoveryinfra "github.com/psyb0t/go-workflows/tests/testinfra/recovery"
	"github.com/psyb0t/go-workflows/workflow"
)

func TestActivityLeaseAndCrashRecovery(t *testing.T) {
	for _, kind := range []testinfra.Kind{testinfra.Postgres, recoveryinfra.RecoverySQLite} {
		t.Run(string(kind), func(t *testing.T) {
			f := newFixture(t)
			f.setBlock(true)
			config, b, api := setup(t, kind, f)
			config.Name = "owner-a"
			a := startWorker(t, config)
			input := inputFor(config)
			instance := submit(t, config, api, input)
			require.Eventually(t, func() bool { return f.count(input.ID, secondStep) == 1 }, testTimeout, pollInterval)
			config.Name = "owner-b"
			startWorker(t, config)
			// A is alive beyond the original lease. B must not steal its heartbeating activity.
			require.Never(t, func() bool { return f.count(input.ID, secondStep) > 1 }, 2*recoveryinfra.RecoveryLease, pollInterval)
			a.kill(t)
			f.setBlock(false)
			result, err := client.GetWorkflowResult[recoveryinfra.RecoveryResult](context.Background(), api, instance, testTimeout)
			require.NoError(t, err)
			require.Equal(t, "owner-a", result.First)
			require.Equal(t, "owner-a", result.Second, "the external fixture deduplicates the already-applied side effect")
			require.Equal(t, 1, f.count(input.ID, firstStep), "checkpointed activity must not execute during replay")
			require.Equal(t, 2, f.count(input.ID, secondStep), "uncheckpointed activity must be delivered again")
			f.mu.Lock()
			workers := append([]string(nil), f.calls[input.ID+"/"+secondStep]...)
			effects := len(f.effects)
			maxActive := f.maxActive[input.ID+"/"+secondStep]
			f.mu.Unlock()
			require.Equal(t, []string{"owner-a", "owner-b"}, workers)
			require.Equal(t, 2, effects, "one logical effect per step")
			require.Equal(t, 1, maxActive)
			waitHistory(t, b, instance, history.EventType_ActivityCompleted, 2)
		})
	}
}

func TestPersistedWaitRecovery(t *testing.T) {
	for _, kind := range []testinfra.Kind{testinfra.Postgres, recoveryinfra.RecoverySQLite} {
		for _, mode := range []string{"signal-before-restart", "signal-after-restart", "timer", "cancel"} {
			t.Run(string(kind)+"/"+mode, func(t *testing.T) {
				f := newFixture(t)
				config, b, api := setup(t, kind, f)
				config.Name = "before-restart"
				a := startWorker(t, config)
				input := inputFor(config)
				input.Wait = "signal"
				if mode == "timer" {
					input.Wait, input.Delay = "timer", 2*time.Second
				}
				instance := submit(t, config, api, input)
				waitHistory(t, b, instance, history.EventType_ActivityCompleted, 1)
				if mode == "timer" {
					waitHistory(t, b, instance, history.EventType_TimerScheduled, 1)
				}
				a.kill(t)
				require.NoError(t, b.Close())
				reopened, err := recoveryinfra.OpenRecoveryBackend(config, false)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, reopened.Close()) })
				api = client.New(reopened)
				if mode == "signal-before-restart" {
					require.NoError(t, api.SignalWorkflow(context.Background(), input.ID, recoveryinfra.RecoverySignal, "persisted"))
				}
				if mode == "cancel" {
					require.NoError(t, api.CancelWorkflowInstance(context.Background(), instance))
				}
				config.Name = "after-restart"
				startWorker(t, config)
				if mode == "signal-after-restart" {
					require.NoError(t, api.SignalWorkflow(context.Background(), input.ID, recoveryinfra.RecoverySignal, "persisted"))
				}
				result, err := client.GetWorkflowResult[recoveryinfra.RecoveryResult](context.Background(), api, instance, testTimeout)
				if mode == "cancel" {
					require.Error(t, err)
					require.True(t, errors.Is(err, workflow.Canceled) || errors.Is(err, client.ErrWorkflowCanceled), "%v", err)
					require.Zero(t, f.count(input.ID, secondStep))
					return
				}
				require.NoError(t, err)
				require.Equal(t, "before-restart", result.First)
				require.Equal(t, "after-restart", result.Second)
				require.Equal(t, 1, f.count(input.ID, firstStep))
				require.Equal(t, 1, f.count(input.ID, secondStep))
				if mode == "timer" {
					waitHistory(t, reopened, instance, history.EventType_TimerFired, 1)
					return
				}
				require.Equal(t, "persisted", result.Signal)
				waitHistory(t, reopened, instance, history.EventType_SignalReceived, 1)
			})
		}
	}
}

func TestQueueRoutingAcrossWorkerRoles(t *testing.T) {
	f := newFixture(t)
	config, _, api := setup(t, testinfra.Postgres, f)
	config.ActivityQueue = config.WorkflowQueue + "-activities"
	input := inputFor(config)
	instance := submit(t, config, api, input)
	wrong := config
	wrong.Name, wrong.WorkflowQueue, wrong.ActivityQueue = "wrong-queue", "unrelated", "unrelated"
	startWorker(t, wrong)
	require.Never(t, func() bool { return f.count(input.ID, firstStep) != 0 }, 300*time.Millisecond, pollInterval)
	state, err := api.GetWorkflowInstanceState(context.Background(), instance)
	require.NoError(t, err)
	require.Equal(t, core.WorkflowInstanceStateActive, state)
	config.Name, config.Role = "orchestrator", recoveryinfra.RecoveryRoleWorkflow
	startWorker(t, config)
	config.Name, config.Role = "activity-only", recoveryinfra.RecoveryRoleActivity
	activities := startWorker(t, config)
	result, err := client.GetWorkflowResult[recoveryinfra.RecoveryResult](context.Background(), api, instance, testTimeout)
	require.NoError(t, err)
	require.Equal(t, "activity-only", result.First)
	require.Equal(t, "activity-only", result.Second)
	activities.stop(t)
}

func TestCompetingReplicas(t *testing.T) {
	f := newFixture(t)
	config, _, api := setup(t, testinfra.Postgres, f)
	const instances = 16
	inputs := make([]recoveryinfra.RecoveryInput, instances)
	workflows := make([]*workflow.Instance, instances)
	for index := range inputs {
		inputs[index] = inputFor(config)
		workflows[index] = submit(t, config, api, inputs[index])
	}
	config.Name = "replica-a"
	a := startWorker(t, config)
	config.Name = "replica-b"
	b := startWorker(t, config)
	for index, instance := range workflows {
		_, err := client.GetWorkflowResult[recoveryinfra.RecoveryResult](context.Background(), api, instance, testTimeout)
		require.NoError(t, err)
		for _, step := range []string{firstStep, secondStep} {
			require.Equal(t, 1, f.count(inputs[index].ID, step))
			f.mu.Lock()
			active := f.maxActive[inputs[index].ID+"/"+step]
			f.mu.Unlock()
			require.Equal(t, 1, active)
		}
	}
	a.stop(t)
	b.stop(t)
}

func TestActivityRetryPolicies(t *testing.T) {
	testCases := []struct {
		name                      string
		failures, attempts, limit int
		permanent, success        bool
		deadline                  time.Duration
	}{
		{name: "transient", failures: 2, limit: 3, attempts: 3, success: true},
		{name: "exhausted", failures: 10, limit: 3, attempts: 3},
		{name: "permanent", permanent: true, limit: 3, attempts: 1},
		{name: "one attempt", failures: 2, limit: 1, attempts: 1},
		{name: "retry deadline", failures: 2, limit: 3, attempts: 1, deadline: time.Millisecond},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.failures, f.permanent = tc.failures, tc.permanent
			config, _, api := setup(t, testinfra.Postgres, f)
			config.Name = "retry-worker"
			startWorker(t, config)
			input := inputFor(config)
			input.Retry.MaxAttempts, input.Retry.RetryTimeout = tc.limit, tc.deadline
			instance := submit(t, config, api, input)
			_, err := client.GetWorkflowResult[recoveryinfra.RecoveryResult](context.Background(), api, instance, testTimeout)
			if tc.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			if tc.permanent {
				require.False(t, workflow.CanRetry(err))
			}
			require.Equal(t, tc.attempts, f.count(input.ID, secondStep))
			require.Equal(t, 1, f.count(input.ID, firstStep))
		})
	}
}

func TestRetryBackoffSurvivesWorkerDeath(t *testing.T) {
	f := newFixture(t)
	f.failures = 1
	config, b, api := setup(t, testinfra.Postgres, f)
	config.Name = "retry-before"
	a := startWorker(t, config)
	input := inputFor(config)
	input.Retry.FirstRetryInterval = time.Second
	instance := submit(t, config, api, input)
	waitHistory(t, b, instance, history.EventType_ActivityFailed, 1)
	waitHistory(t, b, instance, history.EventType_TimerScheduled, 1)
	a.kill(t)
	config.Name = "retry-after"
	startWorker(t, config)
	result, err := client.GetWorkflowResult[recoveryinfra.RecoveryResult](context.Background(), api, instance, testTimeout)
	require.NoError(t, err)
	require.Equal(t, "retry-before", result.First)
	require.Equal(t, "retry-after", result.Second)
	require.Equal(t, 1, f.count(input.ID, firstStep))
	require.Equal(t, 2, f.count(input.ID, secondStep))
}
