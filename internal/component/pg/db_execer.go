package pgdb

import (
	"context"
	"database/sql"
)

type stakeExecer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
}
