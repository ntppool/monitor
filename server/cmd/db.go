package cmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.ntppool.org/monitor/ntpdb"
)

type dbCmd struct {
	Mon dbMonitorCmd `cmd:"" help:"monitor config debug"`
}

type dbMonitorCmd struct {
	Name string `arg:"" help:"monitor name"`
}

func (cmd *dbMonitorCmd) Run(ctx context.Context) error {
	name := cmd.Name
	if name == "" {
		return fmt.Errorf("db mon [monitername]")
	}

	dbconn, err := ntpdb.OpenDB()
	if err != nil {
		return err
	}
	db := ntpdb.New(dbconn)

	mons, err := db.GetMonitorsTLSName(ctx, sql.NullString{String: name, Valid: true})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Println("No monitor found")
			return nil
		}
		return err
	}

	for _, mon := range mons {
		fmt.Printf("Monitor: %+v\n", mon)
		smon, err := ntpdb.GetSystemMonitor(ctx, db, "settings", mon.IpVersion)
		if err == nil {
			mconf, err := mon.GetConfigWithDefaults([]byte(smon.Config))
			if err != nil {
				return err
			}
			fmt.Printf("mconf: %+v", mconf)
		}
	}

	return nil
}
