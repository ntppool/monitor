package ntpdb

//go:generate go tool github.com/hexdigest/gowrap/cmd/gowrap gen -t ./opentelemetry.gowrap -g -i QuerierTx -p . -o otel.go

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.ntppool.org/common/database/pgdb"
)

// OpenDB opens a PostgreSQL connection pool using the specified config file
func OpenDB(ctx context.Context, configFile string) (*pgxpool.Pool, error) {
	return pgdb.OpenPoolWithConfigFile(ctx, configFile)
}
