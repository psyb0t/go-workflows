package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"runtime/coverage"
	"syscall"

	"github.com/psyb0t/go-workflows/tests/testinfra/recovery"
)

// A regular covered binary permits snapshots before SIGKILL. Go test binaries
// initialize their runtime coverage metadata only during test teardown.
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	requests := make(chan os.Signal, 1)
	signal.Notify(requests, syscall.SIGUSR1)
	defer signal.Stop(requests)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			return
		case <-requests:
			dir := os.Getenv("GO_WORKFLOWS_COVDATA_DIR")
			err := errors.Join(coverage.WriteMetaDir(dir), coverage.WriteCountersDir(dir))
			result := "ok"
			if err != nil {
				result = err.Error()
			}
			ack := os.Getenv("GO_WORKFLOWS_COVERAGE_ACK")
			if err := os.WriteFile(ack+".pending", []byte(result), 0o600); err != nil { // #nosec G703 -- Parent supplies a private test artifact path.
				slog.Error("write coverage acknowledgement", "err", err)
				return
			}
			if err := os.Rename(ack+".pending", ack); err != nil { // #nosec G703 -- Atomic publication under the private test directory.
				slog.Error("publish coverage acknowledgement", "err", err)
			}
		}
	}()
	err := recovery.RunRecoveryWorker(ctx, os.Stdin, os.Stdout)
	cancel()
	<-done
	if err != nil {
		slog.Error("recovery worker failed", "err", err)
		os.Exit(1)
	}
}
