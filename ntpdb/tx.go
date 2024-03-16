package ntpdb

import (
	"context"
	"database/sql"
	"fmt"

	"go.ntppool.org/common/logger"
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

	// log := logger.Setup()
	// log.Warn("db type test", "db", q.db, "type", fmt.Sprintf("%T", q.db))

	if db, ok := q.db.(*sql.DB); ok {
		tx, err := db.BeginTx(ctx, &sql.TxOptions{})
		if err != nil {
			return nil, err
		}
		return &Queries{db: tx}, nil
	} else {
		tx, err := q.db.(Beginner).Begin(ctx)
		if err != nil {
			return nil, err
		}
		return &Queries{db: &tx}, nil
	}
}

func (q *Queries) Commit(ctx context.Context) error {
	if db, ok := q.db.(*sql.Tx); ok {
		return db.Commit()
	}

	tx, ok := q.db.(Tx)
	if !ok {
		log := logger.FromContext(ctx)
		log.ErrorContext(ctx, "could not get a Tx", "type", fmt.Sprintf("%T", q.db))
		return sql.ErrTxDone
	}
	return tx.Commit(ctx)
}

func (q *Queries) Rollback(ctx context.Context) error {
	if db, ok := q.db.(*sql.Tx); ok {
		return db.Rollback()
	}

	tx, ok := q.db.(Tx)
	if !ok {
		return sql.ErrTxDone
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
