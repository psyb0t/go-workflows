package registry

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegistrationRejectsInvalidValues(t *testing.T) {
	t.Parallel()
	var nilFunction func() error
	var nilPointer *struct{}
	for _, tc := range []struct {
		name  string
		value any
	}{
		{name: "nil"},
		{name: "typed nil function", value: nilFunction},
		{name: "typed nil pointer", value: nilPointer},
		{name: "string", value: "not a function"},
		{name: "integer", value: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := New()
			var invalidWorkflow *ErrInvalidWorkflow
			var invalidActivity *ErrInvalidActivity
			require.NotPanics(t, func() {
				require.ErrorAs(t, r.RegisterWorkflow(tc.value), &invalidWorkflow)
				require.ErrorAs(t, r.RegisterActivity(tc.value), &invalidActivity)
			})
		})
	}
}
