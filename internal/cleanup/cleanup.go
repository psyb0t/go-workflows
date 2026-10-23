package cleanup

import (
	"context"
	"database/sql"
	"errors"
	"io"

	"github.com/psyb0t/ctxerrors"
	"github.com/psyb0t/ctxscope"
)

// Close reports cleanup failures without replacing the operation's result.
func Close(ctx context.Context, closer io.Closer) {
	if err := closer.Close(); err != nil {
		ctxscope.GetLogger(ctx).Warn("resource close failed", "err", ctxerrors.Wrap(err, "close resource"))
	}
}

// Rollback releases unfinished transactions. A prior commit is not a failure.
func Rollback(ctx context.Context, tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		ctxscope.GetLogger(ctx).Warn("transaction rollback failed", "err", ctxerrors.Wrap(err, "rollback transaction"))
	}
}
