package scripts

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReleaseTag(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name, tag, moduleSuffix string
		wantSuccess             bool
	}{
		{name: "root", tag: "v1.0.0", wantSuccess: true},
		{name: "analyzer", tag: "analyzer/v1.2.3", wantSuccess: true},
		{name: "prerelease and metadata", tag: "v1.0.0-rc.0+build.01", wantSuccess: true},
		{name: "zero major", tag: "v0.9.0", wantSuccess: true},
		{name: "root malformed", tag: "vnext"},
		{name: "analyzer malformed", tag: "analyzer/vdraft"},
		{name: "leading zero", tag: "v01.0.0"},
		{name: "numeric prerelease zero", tag: "v1.0.0-01"},
		{name: "empty prerelease identifier", tag: "v1.0.0-alpha..1"},
		{name: "empty metadata", tag: "v1.0.0+"},
		{name: "root incompatible module", tag: "v2.0.0"},
		{name: "analyzer incompatible module", tag: "analyzer/v2.0.0"},
		{name: "matching root major", tag: "v2.0.0", moduleSuffix: "/v2", wantSuccess: true},
		{name: "matching analyzer major", tag: "analyzer/v2.0.0", moduleSuffix: "/v2", wantSuccess: true},
		{name: "old stable tag after migration", tag: "v1.0.0", moduleSuffix: "/v2"},
		{name: "old unstable tag after migration", tag: "v0.9.0", moduleSuffix: "/v2"},
		{name: "wrong migrated major", tag: "v3.0.0", moduleSuffix: "/v2"},
		{name: "shell input", tag: "v1.0.0;touch"},
		{name: "path traversal", tag: "../v1.0.0"},
		{name: "empty"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "analyzer"), 0o700))
			for _, name := range []string{"check-release-tag.sh", "common.sh"} {
				data, err := os.ReadFile(name) // #nosec G304 -- Fixed production fixture names.
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", name), data, 0o600))
			}
			for _, module := range []string{"go.mod", "analyzer/go.mod"} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, module), []byte("module example.org/workflows"+tc.moduleSuffix+"\n"), 0o600))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			t.Cleanup(cancel)
			command := exec.CommandContext(ctx, "bash", filepath.Join(dir, "scripts", "check-release-tag.sh")) // #nosec G204 -- Executes a private copy of the production script, without shell interpolation.
			command.Env = append(os.Environ(), "RELEASE_TAG="+tc.tag, "GITHUB_REF_NAME=", "LOG_FILE="+filepath.Join(dir, "run.log"))
			output, err := command.CombinedOutput()
			if tc.wantSuccess {
				require.NoError(t, err, "%s", output)
				return
			}
			require.Error(t, err, "%s", output)
			var exitError *exec.ExitError
			require.ErrorAs(t, err, &exitError)
			require.Equal(t, 2, exitError.ExitCode(), "%s", output)
		})
	}
}
