package ntpdb

//go:generate go tool github.com/hexdigest/gowrap/cmd/gowrap gen -t ./opentelemetry.gowrap -g -i QuerierTx -p . -o otel.go

import (
	"context"
	"database/sql"

	"go.ntppool.org/common/database"
)

// Config and DBConfig types are now provided by the common database package

func OpenDB() (*sql.DB, error) {
	options := database.MonitorConfigOptions()
	return database.OpenDB(context.Background(), options)
}

// openDB function is now provided by the common database package

// createConnector function is now provided by the common database package
