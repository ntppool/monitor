package ntpdb

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
)

// from https://github.com/Boostport/dynamic-database-config

type CreateConnectorFunc func() (driver.Connector, error)

type Driver struct {
	CreateConnectorFunc CreateConnectorFunc
}

func (d Driver) Driver() driver.Driver {
	return d
}

func (d Driver) Connect(ctx context.Context) (driver.Conn, error) {
	connector, err := d.CreateConnectorFunc()

	if err != nil {
		return nil, fmt.Errorf("error creating connector from function: %w", err)
	}

	return connector.Connect(ctx)
}

func (d Driver) Open(name string) (driver.Conn, error) {
	return nil, errors.New("open is not supported")
}
