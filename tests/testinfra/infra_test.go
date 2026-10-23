package testinfra

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStart(t *testing.T) {
	t.Parallel()
	for _, kind := range []Kind{"", "unknown", "mysql;exit"} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			infra, err := Start(context.Background(), kind)
			require.Nil(t, infra)
			require.ErrorIs(t, err, errors.ErrUnsupported)
		})
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	i := &Infra{}
	for range 3 {
		require.NoError(t, i.Close())
	}
}

func TestAddress(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ host, expected string }{
		{host: "127.0.0.1", expected: "127.0.0.1:12345"},
		{host: "::1", expected: "[::1]:12345"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			i := &Infra{Host: tc.host, Port: 12345}
			require.Equal(t, tc.expected, i.Address())
		})
	}
}
