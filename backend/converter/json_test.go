package converter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONConverterBoundaries(t *testing.T) {
	t.Parallel()
	converter := &jsonConverter{}
	_, err := converter.To(make(chan int))
	require.Error(t, err)
	var value int
	require.Error(t, converter.From([]byte("{"), &value))
	payload, err := converter.To(42)
	require.NoError(t, err)
	require.NoError(t, converter.From(payload, &value))
	require.Equal(t, 42, value)
}
