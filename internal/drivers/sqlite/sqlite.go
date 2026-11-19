// Package sqlite adapts SQLite files using a pure-Go engine (no CGO needed),
// which also covers DuckDB-free local analytics, app databases, and test fixtures.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

func init() {
	driver.Register(&Driver{})
}

type Driver struct{}

func (Driver) Info() driver.Info {
	return driver.Info{
		ID: "sqlite", Name: "SQLite", Family: driver.FamilySQL, Language: "sql",
		Fields:         []driver.Field{driver.FieldFile},
		URIPlaceholder: "file:app.db?mode=ro",
		Example:        "SELECT sqlite_version();",
	}
}

func (Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	path := strings.TrimSpace(cfg.File)
	if path == "" {
		path = strings.TrimSpace(cfg.URI)
	}
	if path == "" {
		return nil, errors.New("choose a database file")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite allows one writer; keep it simple and safe.
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	_, _ = db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;")
	return &sqlbase.Engine{DB: db, Dialect: dialect{}}, nil
}

type dialect struct{}

func (dialect) Quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func (dialect) Use(string) string { return "" }

func (dialect) Paginate(l, o int, ord bool) string { return sqlbase.LimitOffset(l, o, ord) }

func (d dialect) Table(path []string) (string, string) {
	if len(path) != 1 {
		return "", ""
	}
	return "", d.Quote(path[0])
}

func (dialect) Children(ctx context.Context, e *sqlbase.Engine, path []string) ([]driver.Node, error) {
	if len(path) != 0 {
		return nil, nil
	}
	pairs, err := sqlbase.Pairs(ctx, e.DB, `SELECT name, type FROM sqlite_master
		WHERE type IN ('table','view') AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return sqlbase.Tables(nil, pairs), nil
}
