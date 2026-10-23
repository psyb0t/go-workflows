package recovery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/psyb0t/ctxerrors/commerr"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/workflow"
)

func TestRecoveryConfiguration(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"", "null", "{", `{"unknown":true}`, `{ } { }`,
		`{"Role":"invalid"}`, `{"Role":"both","Name":"worker"}`,
		`{"Role":"both","CallbackURL":"http://127.0.0.1"}`,
		`{"Role":"both","Name":"worker","CallbackURL":"http://127.0.0.1","Kind":"invalid"}`,
		strings.Repeat(" ", recoveryInputLimit+1),
	} {
		t.Run(input[:min(len(input), 80)], func(t *testing.T) {
			t.Parallel()
			err := RunRecoveryWorker(context.Background(), strings.NewReader(input), io.Discard)
			require.Error(t, err)
		})
	}
	for _, config := range []RecoveryConfig{{Kind: RecoverySQLite}, {Kind: "invalid"}} {
		_, err := OpenRecoveryBackend(config, false)
		require.ErrorIs(t, err, commerr.ErrInvalidArgument)
	}
}

type readyWriter func([]byte) (int, error)

func (write readyWriter) Write(data []byte) (int, error) { return write(data) }

func TestRecoveryWorkerShutdown(t *testing.T) {
	t.Parallel()
	for _, role := range []string{RecoveryRoleBoth, RecoveryRoleWorkflow, RecoveryRoleActivity} {
		for _, failReady := range []bool{false, true} {
			name := role
			if failReady {
				name += "/readiness-failure"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				config := RecoveryConfig{Kind: RecoverySQLite, Name: "fixture", CallbackURL: "http://127.0.0.1", SQLitePath: filepath.Join(t.TempDir(), "worker.sqlite"), Role: role}
				b, err := OpenRecoveryBackend(config, true)
				require.NoError(t, err)
				require.NoError(t, b.Close())
				data, err := json.Marshal(config)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				ready := readyWriter(func(data []byte) (int, error) {
					require.Equal(t, "ready\n", string(data))
					if failReady {
						return 0, io.ErrClosedPipe
					}
					cancel()
					return len(data), nil
				})
				err = RunRecoveryWorker(ctx, strings.NewReader(string(data)), ready)
				if failReady {
					require.ErrorIs(t, err, io.ErrClosedPipe)
					return
				}
				require.NoError(t, err)
			})
		}
	}
}

func TestRecoveryCallback(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, body string
		status     int
		wantErr    error
		permanent  bool
	}{
		{name: "success", status: http.StatusOK, body: `{"Value":"recorded"}`},
		{name: "http failure", status: http.StatusServiceUnavailable, wantErr: commerr.ErrFetchFailed},
		{name: "malformed", status: http.StatusOK, body: "{"},
		{name: "transient", status: http.StatusOK, body: `{"Failure":"retry"}`, wantErr: commerr.ErrFetchFailed},
		{name: "permanent", status: http.StatusOK, body: `{"Failure":"permanent"}`, permanent: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var call RecoveryCall
				if err := json.NewDecoder(r.Body).Decode(&call); err != nil || call.ID != "operation" || call.Step != "first" || call.Worker != "fixture" || r.Method != http.MethodPost {
					t.Error("unexpected recovery callback")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.WriteHeader(tc.status)
				if _, err := io.WriteString(w, tc.body); err != nil {
					t.Errorf("write callback response: %v", err)
				}
			}))
			t.Cleanup(server.Close)
			activity := recoveryActivity{config: RecoveryConfig{Name: "fixture", CallbackURL: server.URL}}
			value, err := activity.record(context.Background(), "operation", "first")
			if tc.name == "success" {
				require.NoError(t, err)
				require.Equal(t, "recorded", value)
				return
			}
			require.Error(t, err)
			if tc.permanent {
				var persisted *workflow.Error
				require.ErrorAs(t, err, &persisted)
				require.True(t, persisted.Permanent)
				require.Error(t, persisted.Cause)
			}
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			}
			require.Equal(t, !tc.permanent, workflow.CanRetry(err))
		})
	}
	t.Run("canceled", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		activity := recoveryActivity{config: RecoveryConfig{CallbackURL: "http://127.0.0.1"}}
		_, err := activity.record(ctx, "operation", "first")
		require.ErrorIs(t, err, context.Canceled)
	})
}
