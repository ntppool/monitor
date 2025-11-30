package ntpdb

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.ntppool.org/common/logger"
	"go.opentelemetry.io/otel/trace"
)

type QuerierTx interface {
	Querier

	Begin(ctx context.Context) (QuerierTx, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error

	// Conn returns the connection used by this transaction
	Conn() *pgx.Conn
}

type Beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

type Tx interface {
	Begin(context.Context) (pgx.Tx, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

func (q *Queries) Begin(ctx context.Context) (QuerierTx, error) {
	tx, err := q.db.(Beginner).Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &Queries{db: tx}, nil
}

func (q *Queries) Commit(ctx context.Context) error {
	tx, ok := q.db.(Tx)
	if !ok {
		// Commit called on Queries with dbpool, so treat as transaction already committed
		return pgx.ErrTxClosed
	}
	return tx.Commit(ctx)
}

func (q *Queries) Conn() *pgx.Conn {
	tx, ok := q.db.(*pgxpool.Tx)
	if !ok {
		logger.Setup().Error("could not get connection from QuerierTx")
		return nil
	}
	return tx.Conn()
}

func (q *Queries) Rollback(ctx context.Context) error {
	tx, ok := q.db.(Tx)
	if !ok {
		// Rollback called on Queries with dbpool, so treat as transaction already committed
		return pgx.ErrTxClosed
	}
	return tx.Rollback(ctx)
}

type WrappedQuerier struct {
	QuerierTxWithTracing
}

func NewWrappedQuerier(q QuerierTx) QuerierTx {
	return &WrappedQuerier{NewQuerierTxWithTracing(q, "")}
}

func (wq *WrappedQuerier) Begin(ctx context.Context) (QuerierTx, error) {
	q, err := wq.QuerierTxWithTracing.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return NewWrappedQuerier(q), nil
}

func (wq *WrappedQuerier) Conn() *pgx.Conn {
	return wq.QuerierTxWithTracing.Conn()
}

// LogRollback logs and performs a rollback if the transaction is still active
func LogRollback(ctx context.Context, tx QuerierTx) {
	if !isInTransaction(tx) {
		return
	}

	log := logger.FromContext(ctx)
	log.WarnContext(ctx, "transaction rollback called on an active transaction")

	// if caller ctx is done we still need rollback to happen
	// so Rollback gets a fresh context with span copied over
	rbCtx := context.Background()
	if span := trace.SpanFromContext(ctx); span != nil {
		rbCtx = trace.ContextWithSpan(rbCtx, span)
	}
	if err := tx.Rollback(rbCtx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		log.ErrorContext(ctx, "rollback failed", "err", err)
	}
}

func isInTransaction(tx QuerierTx) bool {
	if tx == nil {
		return false
	}

	conn := tx.Conn()
	if conn == nil {
		return false
	}

	// 'I' means idle, so if it's not idle, we're in a transaction
	return conn.PgConn().TxStatus() != 'I'
}
