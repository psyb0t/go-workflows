package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCoverageGate(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name, threshold, profile, percentage, filtered, child string
		wantExit                                              int
	}{
		{name: "below ninety", profile: "sample.go:1.1,2.1 89 1\nsample.go:3.1,4.1 11 0\n", percentage: "89.00", wantExit: 1},
		{name: "exact minimum", profile: "sample.go:1.1,2.1 90 1\nsample.go:3.1,4.1 10 0\n", percentage: "90.00"},
		{name: "fraction below", profile: "sample.go:1.1,2.1 8999 1\nsample.go:3.1,4.1 1001 0\n", percentage: "89.99", wantExit: 1},
		{name: "rounding cannot pass the floor", profile: "sample.go:1.1,2.1 899999 1\nsample.go:3.1,4.1 100001 0\n", percentage: "90.00", wantExit: 1},
		{name: "all covered", profile: "sample.go:1.1,2.1 100 1\n", percentage: "100.00"},
		{name: "generated files excluded", profile: "example/mock.gen.go:1.1,2.1 999 0\nexample/gen.go:1.1,2.1 999 0\nsample.go:1.1,2.1 100 1\n", filtered: "sample.go:1.1,2.1 100 1\n", percentage: "100.00"},
		{name: "similar handwritten name retained", profile: "example/handwritten_gen.go:1.1,2.1 11 0\nsample.go:1.1,2.1 89 1\n", percentage: "89.00", wantExit: 1},
		{name: "duplicate blocks merged", profile: "sample.go:1.1,2.1 90 0\nsample.go:1.1,2.1 90 1\nsample.go:3.1,4.1 10 0\n", filtered: "sample.go:1.1,2.1 90 1\nsample.go:3.1,4.1 10 0\n", percentage: "90.00"},
		{name: "child process counters included", profile: "sample.go:1.1,2.1 90 0\nsample.go:3.1,4.1 10 0\n", child: "sample.go:1.1,2.1 90 1\n", filtered: "sample.go:1.1,2.1 90 1\nsample.go:3.1,4.1 10 0\n", percentage: "90.00"},
		{name: "child export failure cannot pass", profile: "sample.go:1.1,2.1 100 1\n", child: "invalid", wantExit: 1},
		{name: "sample and benchmark programs excluded", profile: "example.org/workflows/samples/demo/main.go:1.1,2.1 999 0\nexample.org/workflows/bench/main.go:1.1,2.1 999 0\nexample.org/workflows/backend/source.go:1.1,2.1 100 1\n", filtered: "example.org/workflows/backend/source.go:1.1,2.1 100 1\n", percentage: "100.00"},
		{name: "similarly named library packages retained", profile: "example.org/workflows/internal/samples/source.go:1.1,2.1 11 0\nexample.org/workflows/benchmarks/source.go:1.1,2.1 89 1\n", percentage: "89.00", wantExit: 1},
		{name: "stricter threshold", threshold: "95", profile: "sample.go:1.1,2.1 90 1\nsample.go:3.1,4.1 10 0\n", percentage: "90.00", wantExit: 1},
		{name: "cannot lower floor", threshold: "89", wantExit: 2},
		{name: "invalid", threshold: "invalid", wantExit: 2},
		{name: "zero", threshold: "0", wantExit: 2},
		{name: "too high", threshold: "101", wantExit: 2},
		{name: "empty profile", wantExit: 1},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, name := range []string{"test-coverage.sh", "common.sh"} {
				data, err := os.ReadFile(name) // #nosec G304 -- Fixed production script names copied into a private test directory.
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600))
			}
			// #nosec G306 -- Executable stub private to the test's temporary directory.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "go"), []byte(coverageGo), 0o700))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			command := exec.CommandContext(ctx, "bash", filepath.Join(dir, "test-coverage.sh")) // #nosec G204 -- Fixed script copied into a private directory, with no shell interpolation.
			command.Dir = dir
			command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "MIN_TEST_COVERAGE="+tc.threshold, "PROFILE="+tc.profile, "CHILD_PROFILE="+tc.child, "CALLS="+filepath.Join(dir, "calls"), "LOG_FILE="+filepath.Join(dir, "log"))
			output, err := command.CombinedOutput()
			if tc.wantExit == 0 {
				require.NoError(t, err, "%s", output)
			} else {
				var exit *exec.ExitError
				require.ErrorAs(t, err, &exit, "%s", output)
				require.Equal(t, tc.wantExit, exit.ExitCode(), "%s", output)
			}
			if tc.percentage == "" {
				return
			}
			percentage, err := os.ReadFile(filepath.Join(dir, "coverage-percent.txt")) // #nosec G304 -- Fixed report in the private test directory.
			require.NoError(t, err)
			require.Equal(t, tc.percentage, strings.TrimSpace(string(percentage)))
			profile, err := os.ReadFile(filepath.Join(dir, "coverage.out")) // #nosec G304 -- Fixed report in the private test directory.
			require.NoError(t, err)
			expectedProfile := tc.profile
			if tc.filtered != "" {
				expectedProfile = tc.filtered
			}
			require.True(t, strings.HasPrefix(string(profile), "mode: atomic\n"))
			require.ElementsMatch(t, strings.Split(strings.TrimSpace(expectedProfile), "\n"), strings.Split(strings.TrimSpace(strings.TrimPrefix(string(profile), "mode: atomic\n")), "\n"))
			calls, err := os.ReadFile(filepath.Join(dir, "calls")) // #nosec G304 -- Fixed stub output in the private test directory.
			require.NoError(t, err)
			require.Contains(t, string(calls), "test -race -count=1 -timeout=10m -covermode=atomic -coverpkg=./...")
			require.Contains(t, string(calls), " ./...\n")
		})
	}
}

const coverageGo = `#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >>"$CALLS"
if [[ $1 == list ]]; then
  printf '%s\n' example.org/workflows
fi
if [[ $1 == test ]]; then
  if [[ -n ${CHILD_PROFILE:-} && -n ${GO_WORKFLOWS_COVDATA_DIR:-} ]]; then
    touch "$GO_WORKFLOWS_COVDATA_DIR/covcounters.stub"
  fi
  for arg; do
    if [[ $arg == -coverprofile=* ]]; then
      printf 'mode: atomic\n%s' "$PROFILE" >"${arg#-coverprofile=}"
    fi
  done
fi
if [[ $1 == tool && $2 == covdata && $3 == textfmt ]]; then
  if [[ $CHILD_PROFILE == invalid ]]; then
    exit 1
  fi
  for arg; do
    if [[ $arg == -o=* ]]; then
      printf 'mode: atomic\n%s' "$CHILD_PROFILE" >"${arg#-o=}"
    fi
  done
fi
`
