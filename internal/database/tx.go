package database

import (
	"context"
	"database/sql"
)

type txKey struct{}

type sqlContextExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// RunInTx executes the given function within a database transaction.
// If the context already contains a transaction, it reuses it.
func RunInTx(ctx context.Context, db *sql.DB, fn func(ctx context.Context) error) error {
	if TxFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := db.BeginTx(ctx, nil)
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
	if err := fn(txCtx); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// ExecutorFromContext returns the transaction from the context if it exists,
// otherwise it returns the provided fallback executor (usually *sql.DB).
func ExecutorFromContext(ctx context.Context, fallback sqlContextExecutor) sqlContextExecutor {
	if tx := TxFromContext(ctx); tx != nil {
		return tx
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
