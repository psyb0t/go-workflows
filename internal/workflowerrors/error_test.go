package workflowerrors

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/psyb0t/ctxerrors"

	"github.com/stretchr/testify/require"
)

func TestSerializedErrorBoundaries(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, input string
		valid       bool
	}{
		{name: "null", input: "null"},
		{name: "malformed", input: "{"},
		{name: "wrong shape", input: "[]"},
		{name: "empty object", input: "{}", valid: true},
		{name: "nested Unicode", input: `{"message":"échec","cause":{"message":"故障","permanent":true}}`, valid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var got Error
			var err error
			require.NotPanics(t, func() { err = json.Unmarshal([]byte(tc.input), &got) })
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestPermanentErrorThroughWrappers(t *testing.T) {
	t.Parallel()
	permanent := NewPermanentError(errors.New("refused"))
	for _, tc := range []struct {
		name  string
		err   error
		retry bool
	}{
		{name: "nil", retry: true},
		{name: "ordinary", err: ctxerrors.Wrap(errors.New("retry"), "operation"), retry: true},
		{name: "wrapped permanent", err: ctxerrors.Wrap(permanent, "operation")},
		{name: "nested permanent", err: &Error{Message: "outer", Cause: permanent}},
		{name: "joined permanent", err: errors.Join(errors.New("retry"), permanent)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.retry, CanRetry(tc.err))
			encoded, err := json.Marshal(FromError(tc.err))
			require.NoError(t, err)
			var restored *Error
			require.NoError(t, json.Unmarshal(encoded, &restored))
			require.Equal(t, tc.retry, CanRetry(ToError(restored)))
		})
	}
	require.Nil(t, NewPermanentError(nil))
}

func Test_NewError_Nil(t *testing.T) {
	err := FromError(nil)
	require.Nil(t, err)
}

func Test_NewError_DoesNotWrapAgain(t *testing.T) {
	err := FromError(errors.New("foo"))

	err2 := FromError(err)
	require.NoError(t, errors.Unwrap(err2))
}

func Test_NewError_DoesWrap(t *testing.T) {
	input := errors.New("foo")
	e := FromError(input)

	var expectedType *Error
	require.ErrorAs(t, e, &expectedType)
	require.Error(t, e, input.Error())

	require.False(t, e.Permanent)
	require.NoError(t, e.Cause)
}

func Test_NewPermanentError(t *testing.T) {
	input := errors.New("foo")
	e := NewPermanentError(input)

	var expected *Error
	require.ErrorAs(t, e, &expected)
	require.Error(t, e, input.Error())

	require.True(t, e.Permanent)
	require.NoError(t, e.Cause)
}

func Test_RoundTrip(t *testing.T) {
	input := NewPanicError("foo")
	e := FromError(input)

	output := ToError(e)
	require.Equal(t, input, output)
}

func TestCanRetry(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "Error",
			err:  FromError(errors.New("foo")),
			want: true,
		},
		{
			name: "Permanent",
			err:  NewPermanentError(errors.New("foo")),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanRetry(tt.err); got != tt.want {
				t.Errorf("CanRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}
