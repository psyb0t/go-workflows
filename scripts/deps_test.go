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

func TestDependencyCommand(t *testing.T) {
	testCases := []struct {
		name, action, pkg, module, mode string
		wantSuccess, wantMutation       bool
	}{
		{name: "unpinned", action: "add", pkg: "example.org/lib", module: "."},
		{name: "shell input", action: "add", pkg: "example.org/lib@v1.0.0;touch", module: "."},
		{name: "invalid module", action: "lock", module: "../outside"},
		{name: "unknown action", action: "bogus", module: "."},
		{name: "too new", action: "add", pkg: "example.org/lib@v1.0.0", module: ".", mode: "new"},
		{name: "missing timestamp", action: "add", pkg: "example.org/lib@v1.0.0", module: ".", mode: "missing"},
		{name: "network failure", action: "add", pkg: "example.org/lib@v1.0.0", module: ".", mode: "offline"},
		{name: "all candidates checked first", action: "add", pkg: "example.org/lib@v1.0.0 example.org/new@v1.0.0", module: ".", mode: "mixed"},
		{name: "add pinned", action: "add", pkg: "example.org/lib@v1.0.0", module: ".", wantSuccess: true, wantMutation: true},
		{name: "nested lock", action: "lock", module: "analyzer", wantSuccess: true, wantMutation: true},
		{name: "audit failure", action: "lock", module: ".", mode: "audit", wantMutation: true},
		{name: "upgrade tools too", action: "upgrade", module: ".", wantSuccess: true, wantMutation: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "scripts"), 0o700))
			require.NoError(t, os.MkdirAll(filepath.Join(dir, "analyzer"), 0o700))
			for _, name := range []string{"deps.sh", "common.sh"} {
				data, err := os.ReadFile(name) // #nosec G304 -- Fixed fixture names declared in this test.
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", name), data, 0o600))
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "analyzer", "go.mod"), []byte("module example.org/analyzer\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts", "sec.sh"), []byte("#!/bin/bash\n[[ $MODE != audit ]]\n"), 0o600))
			// #nosec G306 -- Executable fake tool, private to the test's temporary directory.
			require.NoError(t, os.WriteFile(filepath.Join(dir, "go"), []byte(fakeGo), 0o700))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, "bash", filepath.Join(dir, "scripts", "deps.sh"), tc.action) // #nosec G204 -- Executes the copied production command with table-owned arguments, not shell interpolation.
			command.Dir = dir
			command.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "PKG="+tc.pkg, "MODULE_DIR="+tc.module, "MODE="+tc.mode, "CALLS="+filepath.Join(dir, "calls"), "LOG_FILE="+filepath.Join(dir, "log"))
			output, err := command.CombinedOutput()
			if tc.wantSuccess {
				require.NoError(t, err, "%s", output)
			} else {
				require.Error(t, err, "%s", output)
			}
			calls, readErr := os.ReadFile(filepath.Join(dir, "calls")) // #nosec G304 -- Output from the private fake tool.
			if readErr != nil {
				require.True(t, os.IsNotExist(readErr))
			}
			mutated := strings.Contains(string(calls), "get ") || strings.Contains(string(calls), "mod ")
			require.Equal(t, tc.wantMutation, mutated, "%s", calls)
			if tc.action == "upgrade" {
				require.Contains(t, string(calls), "example.org/tool@v1.0.0")
			}
		})
	}
}

const fakeGo = `#!/bin/bash
set -euo pipefail
printf '%s\n' "$*" >>"$CALLS"
case "$*" in
  'list -f {{.Module.Path}} tool') printf 'example.org/tool\n';;
  'list -m -json all') printf '{"Path":"example.org/lib"}\n{"Path":"example.org/tool","Indirect":true}\n';;
  'list -m -f {{.Version}} '*) printf 'v1.0.0\n';;
  'list -m -json '*)
    [[ $MODE != offline ]] || exit 1
    timestamp=2000-01-01T00:00:00Z
    if [[ $MODE == new || ($MODE == mixed && $* == *example.org/new*) ]]; then timestamp=$(date -u +%FT%TZ); fi
    if [[ $MODE == missing ]]; then printf '{"Version":"v1.0.0"}\n'; else printf '{"Version":"v1.0.0","Time":"%s"}\n' "$timestamp"; fi;;
esac
`
