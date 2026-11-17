package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// stmtCache is a transaction whose statements are each prepared the first time
// they run and kept until it ends, so a write that runs a statement many times
// prepares it once, and a statement it never runs is never prepared. It is the
// [sqlcdb.DBTX] of [Store.inPreparedTx]; nothing else may use it, and only one
// goroutine at a time, as a transaction allows.
type stmtCache struct {
	tx    *sql.Tx
	stmts map[string]*sql.Stmt
}

func newStmtCache(tx *sql.Tx) *stmtCache {
	return &stmtCache{tx: tx, stmts: map[string]*sql.Stmt{}}
}

func (c *stmtCache) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	st, err := c.stmt(ctx, query) //nolint:sqlclosecheck // The cache owns it: close shuts it when the transaction ends.
	if err != nil {
		return nil, err
	}
	return st.ExecContext(ctx, args...) //nolint:wrapcheck // As the transaction's own would.
}

func (c *stmtCache) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	st, err := c.stmt(ctx, query) //nolint:sqlclosecheck // The cache owns it: close shuts it when the transaction ends.
	if err != nil {
		return nil, err
	}
	return st.QueryContext(ctx, args...) //nolint:wrapcheck // As the transaction's own would.
}

// QueryRowContext returns the row, or a [*sql.Row] whose Scan reports a failed
// prepare: a Row can't be built with an error, so the transaction's own
// QueryRowContext, which prepares again, says what failed.
func (c *stmtCache) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	st, err := c.stmt(ctx, query) //nolint:sqlclosecheck // The cache owns it: close shuts it when the transaction ends.
	if err != nil {
		return c.tx.QueryRowContext(ctx, query, args...)
	}
	return st.QueryRowContext(ctx, args...)
}

func (c *stmtCache) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return c.stmt(ctx, query)
}

// stmt returns query prepared on the transaction, preparing it the first time.
func (c *stmtCache) stmt(ctx context.Context, query string) (*sql.Stmt, error) {
	if st, ok := c.stmts[query]; ok {
		return st, nil
	}
	st, err := c.tx.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("prepare: %w", err)
	}
	c.stmts[query] = st
	return st, nil
}

// close closes every statement prepared.
func (c *stmtCache) close() error {
	errs := make([]error, 0, len(c.stmts))
	for _, st := range c.stmts {
		errs = append(errs, st.Close())
	}
	clear(c.stmts)
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("close statements: %w", err)
	}
	return nil
}
