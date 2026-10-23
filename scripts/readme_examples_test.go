package scripts

import (
	"go/format"
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadmeExamplesMatchRunnableSamples(t *testing.T) {
	t.Parallel()
	readme, err := os.ReadFile("../README.md")
	require.NoError(t, err)
	examples := regexp.MustCompile("(?s)```go\\n(.*?)\\n```").FindAllSubmatch(readme, -1)
	paths := []string{"../samples/greeting/main.go", "../samples/quickstart/main.go"}
	require.Len(t, examples, len(paths), "keep both README programs complete and executable")
	for i, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			source, err := os.ReadFile(path) // #nosec G304 -- Fixed sample paths, not external input.
			require.NoError(t, err)
			formatted, err := format.Source(examples[i][1])
			require.NoError(t, err)
			want, err := format.Source(source)
			require.NoError(t, err)
			require.Equal(t, string(want), string(formatted), "update README and its tested sample together")
		})
	}
}
