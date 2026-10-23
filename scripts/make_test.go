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

func TestMakeCPULimits(t *testing.T) {
	t.Parallel()
	makefile, err := os.ReadFile("../Makefile")
	require.NoError(t, err)
	testCases := []struct {
		name, ci, environment, localFile, want string
	}{
		{name: "default has no cap"},
		{name: "local environment", environment: "2.5", want: "--cpus \"2.5\""},
		{name: "local env file", localFile: "DEV_CPUS=3\n", want: "--cpus \"3\""},
		{name: "CI ignores environment cap", ci: "true", environment: "2"},
		{name: "CI ignores local env file", ci: "true", localFile: "DEV_CPUS=3\n"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), makefile, 0o600))
			if tc.localFile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(tc.localFile), 0o600))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			command := exec.CommandContext(ctx, "make", "--no-print-directory", "-n", "shell", "test", "build-ui")
			command.Dir = dir
			for _, entry := range os.Environ() {
				if strings.HasPrefix(entry, "CI=") || strings.HasPrefix(entry, "DEV_CPUS=") || strings.HasPrefix(entry, "MAKEFLAGS=") {
					continue
				}
				command.Env = append(command.Env, entry)
			}
			command.Env = append(command.Env, "CI="+tc.ci)
			if tc.environment != "" {
				command.Env = append(command.Env, "DEV_CPUS="+tc.environment)
			}
			output, err := command.CombinedOutput()
			require.NoError(t, err, "%s", output)
			var runs int
			for line := range strings.SplitSeq(string(output), "\n") {
				if !strings.HasPrefix(line, "docker run ") {
					continue
				}
				runs++
				if strings.HasSuffix(line, " bash") {
					require.Contains(t, line, "--interactive --tty")
				} else {
					require.NotContains(t, line, "--interactive")
				}
				if tc.want == "" {
					require.NotContains(t, line, "--cpus")
					continue
				}
				require.Contains(t, line, tc.want)
			}
			require.Equal(t, 4, runs)
		})
	}
}
