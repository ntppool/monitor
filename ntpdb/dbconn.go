package ntpdb

//go:generate go tool github.com/hexdigest/gowrap/cmd/gowrap gen -t opentelemetry -i QuerierTx -p . -o otel.go

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
)

type DBConfig struct {
	DSN  string `default:"" flag:"dsn" usage:"Database DSN"`
	User string `default:"" flag:"user"`
	Pass string `default:"" flag:"pass"`
}

func OpenDB(config DBConfig) (*sql.DB, error) {

	dbconn := sql.OpenDB(Driver{CreateConnectorFunc: createConnector(config)})

	dbconn.SetConnMaxLifetime(time.Minute * 3)
	dbconn.SetMaxOpenConns(10)
	dbconn.SetMaxIdleConns(5)

	err := dbconn.Ping()
	if err != nil {
		return nil, err
	}

	return dbconn, nil
}

func createConnector(cfg DBConfig) CreateConnectorFunc {
	return func() (driver.Connector, error) {
		dsn := cfg.DSN
		if len(dsn) == 0 {
			return nil, fmt.Errorf("--database.dsn flag or DATABASE_DSN environment variable required")
		}

		dbcfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, err
		}

		if user := cfg.User; len(user) > 0 {
			dbcfg.User = user
		}

		if pass := cfg.Pass; len(pass) > 0 {
			dbcfg.Passwd = pass
		}

		return mysql.NewConnector(dbcfg)
	}
}
