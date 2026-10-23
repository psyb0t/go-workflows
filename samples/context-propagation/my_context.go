package main

import (
	"context"
	"encoding/json"

	"github.com/psyb0t/ctxerrors"

	"github.com/psyb0t/go-workflows/workflow"
)

type myKey int

var k myKey

type myData struct {
	Name  string
	Count int
}

func withMyValues(ctx context.Context, d *myData) context.Context {
	return context.WithValue(ctx, k, d)
}

func myValues(ctx context.Context) *myData {
	return ctx.Value(k).(*myData)
}

func withMyValuesWf(ctx workflow.Context, d *myData) workflow.Context {
	return workflow.WithValue(ctx, k, d)
}

func myValuesWf(ctx workflow.Context) *myData {
	return ctx.Value(k).(*myData)
}

// myPropagator copies only explicitly selected data, not arbitrary context values.
// Configure it on every producer and consumer of this backend. Never include secrets.
type myPropagator struct{}

var _ workflow.ContextPropagator = &myPropagator{}

// Extract restores metadata into the ordinary Go context used by an activity.
func (*myPropagator) Extract(ctx context.Context, metadata *workflow.Metadata) (context.Context, error) {
	ms := metadata.Get("mydata")

	var d *myData
	if err := json.Unmarshal([]byte(ms), &d); err != nil {
		return nil, ctxerrors.Wrap(err, "execute my context")
	}

	return withMyValues(ctx, d), nil
}

// ExtractToWorkflow restores the same values into the replay-safe workflow context.
func (*myPropagator) ExtractToWorkflow(ctx workflow.Context, metadata *workflow.Metadata) (workflow.Context, error) {
	ms := metadata.Get("mydata")

	var d *myData
	if err := json.Unmarshal([]byte(ms), &d); err != nil {
		return nil, ctxerrors.Wrap(err, "execute my context")
	}

	return withMyValuesWf(ctx, d), nil
}

// Inject records client or activity context values before a task crosses storage.
func (*myPropagator) Inject(ctx context.Context, metadata *workflow.Metadata) error {
	d := myValues(ctx)
	b, err := json.Marshal(d)
	if err != nil {
		return ctxerrors.Wrap(err, "execute my context")
	}

	metadata.Set("mydata", string(b))

	return nil
}

// InjectFromWorkflow carries changed workflow values into subsequently scheduled tasks.
func (*myPropagator) InjectFromWorkflow(ctx workflow.Context, metadata *workflow.Metadata) error {
	d := myValuesWf(ctx)
	b, err := json.Marshal(d)
	if err != nil {
		return ctxerrors.Wrap(err, "execute my context")
	}

	metadata.Set("mydata", string(b))

	return nil
}
