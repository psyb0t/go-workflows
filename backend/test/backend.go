package test

import (
	"context"

	"github.com/psyb0t/go-workflows/backend"
	"github.com/psyb0t/go-workflows/backend/history"
)

type TestBackend interface {
	backend.Backend

	GetFutureEvents(ctx context.Context) ([]*history.Event, error)
}
