package cmd

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"log"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.ntppool.org/monitor/ntpdb"
)

func (cli *CLI) OpenDB() (*sql.DB, error) {

	dbconn := sql.OpenDB(ntpdb.Driver{CreateConnectorFunc: createConnector(cli.Config)})

	dbconn.SetConnMaxLifetime(time.Minute * 3)
	dbconn.SetMaxOpenConns(10)
	dbconn.SetMaxIdleConns(5)

	err := dbconn.Ping()
	if err != nil {
		log.Printf("Could not connect to database: %s", err)
		return nil, err
	}

	return dbconn, nil
}

func createConnector(cfg *APIConfig) ntpdb.CreateConnectorFunc {
	return func() (driver.Connector, error) {
		dsn := cfg.Database.DSN
		if len(dsn) == 0 {
			return nil, fmt.Errorf("--database.dsn flag or DATABASE_DSN environment variable required")
		}

		dbcfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, err
		}

		if user := cfg.Database.User; len(user) > 0 && err == nil {
			dbcfg.User = user
		}

		if pass := cfg.Database.Pass; len(pass) > 0 && err == nil {
			dbcfg.Passwd = pass
		}

		return mysql.NewConnector(dbcfg)
	}
}
