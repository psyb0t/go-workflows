package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
	"github.com/psyb0t/go-workflows/client"
	"github.com/psyb0t/go-workflows/tests/testinfra"
	recoveryinfra "github.com/psyb0t/go-workflows/tests/testinfra/recovery"
	"github.com/psyb0t/go-workflows/workflow"
)

const (
	workerEnvironment      = "GO_WORKFLOWS_TEST_WORKER"
	coverageEnvironment    = "GO_WORKFLOWS_COVDATA_DIR"
	coverageAckEnvironment = "GO_WORKFLOWS_COVERAGE_ACK"
	testTimeout            = 30 * time.Second
	pollInterval           = 20 * time.Millisecond
	firstStep              = "first"
	secondStep             = "second"
)

var infra *testinfra.Infra

func TestMain(m *testing.M) {
	flag.Parse()
	if os.Getenv(workerEnvironment) == "1" {
		os.Exit(m.Run())
	}
	testinfra.Exit(m, testinfra.Postgres, &infra)
}

func TestWorkerProcess(t *testing.T) {
	if os.Getenv(workerEnvironment) != "1" {
		return
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	require.NoError(t, recoveryinfra.RunRecoveryWorker(ctx, os.Stdin, os.Stdout))
}

type process struct {
	command     *exec.Cmd
	done        chan error
	stopped     bool
	coverageAck string
}

func startWorker(t *testing.T, config recoveryinfra.RecoveryConfig) *process {
	t.Helper()
	binary, err := os.Executable()
	require.NoError(t, err)
	data, err := json.Marshal(config)
	require.NoError(t, err)
	artifactDir := filepath.Join("..", "..", ".testing", "process-recovery", uuid.NewString())
	require.NoError(t, os.MkdirAll(artifactDir, 0o700))
	log, err := os.OpenFile(filepath.Join(artifactDir, "worker.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- Fixed filename under a test-owned directory with a generated UUID.
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	arguments := []string{"-test.run=^TestWorkerProcess$"}
	if workerBinary := os.Getenv("GO_WORKFLOWS_COVERAGE_WORKER"); workerBinary != "" {
		binary, arguments = workerBinary, nil
	}
	command := exec.CommandContext(ctx, binary, arguments...) // #nosec G204 -- Executes this test binary or the coverage runner's worker binary.
	command.Env = append(os.Environ(), workerEnvironment+"=1")
	ack := ""
	if testing.CoverMode() != "" && os.Getenv(coverageEnvironment) != "" {
		ack, err = filepath.Abs(filepath.Join(artifactDir, "coverage.ack"))
		require.NoError(t, err)
		command.Env = append(command.Env, coverageAckEnvironment+"="+ack)
	}
	command.Stdin = bytes.NewReader(data)
	command.Stderr = log
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	p := &process{command: command, done: make(chan error, 1), coverageAck: ack}
	t.Cleanup(func() {
		require.NoError(t, log.Close())
		output, readErr := os.ReadFile(filepath.Join(artifactDir, "worker.log")) // #nosec G304 -- Reads only the child log created in this test-owned UUID directory.
		require.NoError(t, readErr)
		require.NotContains(t, string(output), "WARNING: DATA RACE", "child race detector failed; log: %s", artifactDir)
	})
	t.Cleanup(cancel)
	t.Cleanup(func() {
		if !p.stopped && command.Process != nil {
			p.kill(t)
		}
	})
	require.NoError(t, command.Start())
	go func() { p.done <- command.Wait() }()
	ready := make(chan string, 1)
	go func() {
		line, readErr := bufio.NewReader(stdout).ReadString('\n')
		if readErr != nil {
			ready <- "readiness failed"
			return
		}
		ready <- line
	}()
	select {
	case line := <-ready:
		require.Equal(t, "ready\n", line, "worker log: %s", artifactDir)
	case <-ctx.Done():
		t.Fatalf("worker startup timed out; log: %s", artifactDir)
	}
	return p
}

func (p *process) kill(t *testing.T) {
	t.Helper()
	if p.stopped {
		return
	}
	p.flushCoverage(t)
	require.NoError(t, p.command.Process.Kill())
	select {
	case err := <-p.done:
		p.stopped = true
		var exitError *exec.ExitError
		require.ErrorAs(t, err, &exitError)
		status, ok := exitError.Sys().(syscall.WaitStatus)
		require.True(t, ok)
		require.True(t, status.Signaled(), "worker exited before it could be killed: %v", err)
		require.Equal(t, syscall.SIGKILL, status.Signal())
	case <-time.After(testTimeout):
		t.Fatal("killed worker was not reaped")
	}
	p.stopped = true
}

func (p *process) stop(t *testing.T) {
	t.Helper()
	p.flushCoverage(t)
	require.NoError(t, p.command.Process.Signal(os.Interrupt))
	select {
	case err := <-p.done:
		require.NoError(t, err)
	case <-time.After(testTimeout):
		t.Fatal("worker did not stop gracefully")
	}
	p.stopped = true
}

func (p *process) flushCoverage(t *testing.T) {
	t.Helper()
	if p.coverageAck == "" {
		return
	}
	require.NoError(t, p.command.Process.Signal(syscall.SIGUSR1))
	require.Eventually(t, func() bool {
		_, err := os.Stat(p.coverageAck)
		return err == nil
	}, testTimeout, pollInterval)
	result, err := os.ReadFile(p.coverageAck) // #nosec G304 -- Private parent-selected test artifact.
	require.NoError(t, err)
	require.Equal(t, "ok", string(result), "child coverage export failed")
}

type fixture struct {
	mu        sync.Mutex
	server    *httptest.Server
	calls     map[string][]string
	effects   map[string]string
	active    map[string]int
	maxActive map[string]int
	block     bool
	failures  int
	permanent bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{calls: map[string][]string{}, effects: map[string]string{}, active: map[string]int{}, maxActive: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fixture) serve(w http.ResponseWriter, r *http.Request) {
	var call recoveryinfra.RecoveryCall
	if r.Method != http.MethodPost || json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&call) != nil || call.ID == "" || call.Worker == "" || (call.Step != firstStep && call.Step != secondStep) {
		http.Error(w, "invalid fixture request", http.StatusBadRequest)
		return
	}
	key := call.ID + "/" + call.Step
	f.mu.Lock()
	f.calls[key] = append(f.calls[key], call.Worker)
	f.active[key]++
	if f.active[key] > f.maxActive[key] {
		f.maxActive[key] = f.active[key]
	}
	blocked := f.block && call.Step == secondStep
	reply := recoveryinfra.RecoveryReply{}
	if call.Step == secondStep && f.permanent {
		reply.Failure = "permanent"
	} else if call.Step == secondStep && len(f.calls[key]) <= f.failures {
		reply.Failure = "retry"
	} else {
		if f.effects[key] == "" {
			f.effects[key] = call.Worker
		}
		reply.Value = f.effects[key]
	}
	f.mu.Unlock()
	defer func() { f.mu.Lock(); f.active[key]--; f.mu.Unlock() }()
	if blocked {
		<-r.Context().Done()
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(reply); err != nil && !errors.Is(err, context.Canceled) {
		return
	}
}

func (f *fixture) count(id, step string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls[id+"/"+step])
}

func (f *fixture) setBlock(value bool) { f.mu.Lock(); f.block = value; f.mu.Unlock() }

func setup(t *testing.T, kind testinfra.Kind, f *fixture) (recoveryinfra.RecoveryConfig, backend.Backend, *client.Client) {
	t.Helper()
	if testing.Short() && kind == testinfra.Postgres {
		t.Skip("external-backend integration requires the complete suite")
	}
	queue := workflow.Queue(uuid.NewString())
	c := recoveryinfra.RecoveryConfig{Kind: kind, Name: "parent", CallbackURL: f.server.URL, WorkflowQueue: queue, ActivityQueue: queue, Role: recoveryinfra.RecoveryRoleBoth}
	if kind == testinfra.Postgres {
		require.NotNil(t, infra)
		c.Host, c.Port, c.User, c.Password, c.Database = infra.Host, infra.Port, infra.User, infra.Password, "postgres"
	} else {
		c.SQLitePath = filepath.Join(t.TempDir(), "recovery.sqlite")
	}
	b, err := recoveryinfra.OpenRecoveryBackend(c, true)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, b.Close()) })
	return c, b, client.New(b)
}

func submit(t *testing.T, c recoveryinfra.RecoveryConfig, api *client.Client, input recoveryinfra.RecoveryInput) *workflow.Instance {
	t.Helper()
	instance, err := api.CreateWorkflowInstance(context.Background(), client.WorkflowInstanceOptions{InstanceID: input.ID, Queue: c.WorkflowQueue}, recoveryinfra.RecoveryWorkflow, input)
	require.NoError(t, err)
	return instance
}

func waitHistory(t *testing.T, b backend.Backend, instance *workflow.Instance, kind history.EventType, count int) {
	t.Helper()
	require.Eventually(t, func() bool {
		events, err := b.GetWorkflowInstanceHistory(context.Background(), instance, nil)
		if err != nil {
			return false
		}
		found := 0
		for _, event := range events {
			if event.Type == kind {
				found++
			}
		}
		return found >= count
	}, testTimeout, pollInterval)
}

func inputFor(c recoveryinfra.RecoveryConfig) recoveryinfra.RecoveryInput {
	return recoveryinfra.RecoveryInput{ID: uuid.NewString(), Queue: c.ActivityQueue, Retry: workflow.RetryOptions{MaxAttempts: 3, FirstRetryInterval: 50 * time.Millisecond, BackoffCoefficient: 1}}
}
