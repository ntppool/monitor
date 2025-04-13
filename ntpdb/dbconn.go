package ntpdb

//go:generate go tool github.com/hexdigest/gowrap/cmd/gowrap gen -t opentelemetry -i QuerierTx -p . -o otel.go

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.ntppool.org/common/logger"
	"gopkg.in/yaml.v3"
)

type Config struct {
	MySQL DBConfig `yaml:"mysql"`
}

type DBConfig struct {
	DSN    string `default:"" flag:"dsn" usage:"Database DSN"`
	User   string
	Pass   string
	DBName string
}

func OpenDB() (*sql.DB, error) {
	return openDB([]string{
		"database.yaml", "/vault/secrets/database.yaml",
	})
}

func openDB(configFiles []string) (*sql.DB, error) {
	log := logger.Setup()

	var configFile string
	configFiles = append(configFiles, "")

	var err error
	for _, configFile = range configFiles {
		if configFile == "" {
			continue
		}
		if _, sterr := os.Stat(configFile); sterr == nil {
			break
		} else {
			if err == nil {
				err = sterr
			}
		}
	}
	if configFile == "" {
		return nil, fmt.Errorf("no config file found")
	}

	dbconn := sql.OpenDB(Driver{CreateConnectorFunc: createConnector(configFile)})

	dbconn.SetConnMaxLifetime(time.Minute * 3)
	dbconn.SetMaxOpenConns(10)
	dbconn.SetMaxIdleConns(5)

	err = dbconn.Ping()
	if err != nil {
		log.Error("could not connect to database", "err", err)
		return nil, err
	}

	return dbconn, nil
}

func createConnector(configFile string) CreateConnectorFunc {
	// log := logger.Setup()

	return func() (driver.Connector, error) {
		// log.Debug("opening config", "file", configFile)

		dbFile, err := os.Open(configFile)
		if err != nil {
			return nil, err
		}

		dec := yaml.NewDecoder(dbFile)

		cfg := Config{}

		err = dec.Decode(&cfg)
		if err != nil {
			return nil, err
		}

		// log.Printf("db cfg: %+v", cfg)

		dsn := cfg.MySQL.DSN

		if len(dsn) == 0 {
			dsn = os.Getenv("DATABASE_DSN")
			if len(dsn) == 0 {
				return nil, fmt.Errorf("dsn config in database.yaml or DATABASE_DSN environment variable required")
			}
		}

		dbcfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, err
		}

		if user := cfg.MySQL.User; len(user) > 0 {
			dbcfg.User = user
		}

		if pass := cfg.MySQL.Pass; len(pass) > 0 {
			dbcfg.Passwd = pass
		}

		if name := cfg.MySQL.DBName; len(name) > 0 {
			dbcfg.DBName = name
		}

		return mysql.NewConnector(dbcfg)
	}
}
