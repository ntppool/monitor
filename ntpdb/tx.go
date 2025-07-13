package ntpdb

import (
	"context"
	"database/sql"

	"go.ntppool.org/common/database"
)

type QuerierTx interface {
	Querier

	Begin(ctx context.Context) (QuerierTx, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

type Beginner interface {
	Begin(context.Context) (sql.Tx, error)
}

type Tx interface {
	Begin(context.Context) (sql.Tx, error)
	Commit(ctx context.Context) error
	Rollback(ctx context.Context) error
}

func (q *Queries) Begin(ctx context.Context) (QuerierTx, error) {
	result, err := database.BeginTransactionForQuerier(ctx, q.db)
	if err != nil {
		return nil, err
	}
	return &Queries{db: result}, nil
}

func (q *Queries) Commit(ctx context.Context) error {
	return database.CommitTransactionForQuerier(ctx, q.db)
}

func (q *Queries) Rollback(ctx context.Context) error {
	return database.RollbackTransactionForQuerier(ctx, q.db)
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
