package activity

import (
	"context"
	"log/slog"

	"github.com/psyb0t/go-workflows/internal/activity"
	"github.com/psyb0t/go-workflows/internal/log"
)

// Logger returns a logger with the workflow instance this activity is executed for set as default fields
func Logger(ctx context.Context) *slog.Logger {
	return log.WithContext(activity.GetActivityState(ctx).Logger, ctx)
}

// Attempt returns the current attempt of this activity execution
func Attempt(ctx context.Context) int {
	return activity.GetActivityState(ctx).Attempt
}
