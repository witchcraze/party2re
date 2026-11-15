package database

import (
	"context"
	"database/sql"
	"errors"

	"github.com/witchcraze/party2re/internal/economy"
)

type txKey struct{}

type sqlContextExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// RunInTx executes the given function within a database transaction using READ COMMITTED isolation.
// If the context already contains a transaction, it reuses it.
func RunInTx(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	return RunInTxWithOptions(ctx, db, &sql.TxOptions{Isolation: sql.LevelReadCommitted}, fn)
}

// RunInTxWithOptions executes the given function within a database transaction with the specified options.
// If the context already contains an ambient transaction, it reuses it without re-configuring isolation options.
// A changed character-deletion row set restarts only the root callback, up to three attempts.
func RunInTxWithOptions(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(ctx context.Context) error) error {
	if TxFromContext(ctx) != nil {
		return economy.WithRollbackScope(ctx, fn)
	}

	if opts == nil {
		opts = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := runInNewTx(ctx, db, opts, fn)
		if !errors.Is(err, errDeletionRowsChanged) || attempt >= 2 {
			return err
		}
	}
}

func runInNewTx(ctx context.Context, db *sql.DB, opts *sql.TxOptions, fn func(ctx context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	tx, err := conn.BeginTx(context.WithoutCancel(ctx), opts)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	txCtx := context.WithValue(ctx, txKey{}, tx)
	if err := economy.WithRollbackScope(txCtx, func(scopedCtx context.Context) error {
		if err := fn(scopedCtx); err != nil {
			return err
		}
		return scopedCtx.Err()
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

type txExecutor struct {
	tx *sql.Tx
}

func (e txExecutor) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if economy.HasRollbackHooks(ctx) {
		ctx = context.WithoutCancel(ctx)
	}
	return e.tx.ExecContext(ctx, query, args...)
}

func (e txExecutor) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	if ctx.Err() == nil && economy.HasRollbackHooks(ctx) {
		ctx = context.WithoutCancel(ctx)
	}
	return e.tx.QueryRowContext(ctx, query, args...)
}

func (e txExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if economy.HasRollbackHooks(ctx) {
		ctx = context.WithoutCancel(ctx)
	}
	return e.tx.QueryContext(ctx, query, args...)
}

// ExecutorFromContext returns the transaction from the context if it exists,
// otherwise it returns the provided fallback executor (usually *sql.DB).
func ExecutorFromContext(ctx context.Context, fallback sqlContextExecutor) sqlContextExecutor {
	if tx := TxFromContext(ctx); tx != nil {
		return txExecutor{tx: tx}
	}
	return fallback
}

// TxFromContext retrieves the *sql.Tx from the context.
func TxFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(txKey{}).(*sql.Tx)
	return tx
}

// TransactionProvider can be injected into services that need to orchestrate transactions.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type DefaultTransactionProvider struct {
	db *sql.DB
}

func NewTransactionProvider(db *sql.DB) *DefaultTransactionProvider {
	return &DefaultTransactionProvider{db: db}
}

func (p *DefaultTransactionProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return RunInTx(ctx, p.db, fn)
}
