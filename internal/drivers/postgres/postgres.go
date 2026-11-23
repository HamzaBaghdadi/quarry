// Package postgres adapts PostgreSQL and wire-compatible engines
// (CockroachDB, TimescaleDB, Supabase, Neon, YugabyteDB, …).
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

var fields = []driver.Field{driver.FieldHost, driver.FieldPort, driver.FieldUser, driver.FieldPassword, driver.FieldDatabase, driver.FieldSSL, driver.FieldURI}

func init() {
	driver.Register(&Driver{info: driver.Info{
		ID: "postgres", Name: "PostgreSQL", Family: driver.FamilySQL, Language: "sql",
		DefaultPort: 5432, DefaultUser: "postgres", Fields: fields,
		SSLModes:       []string{"prefer", "disable", "require", "verify-ca", "verify-full"},
		URIPlaceholder: "postgres://user:pass@localhost:5432/db?sslmode=disable",
		Example:        "SELECT now(), version();",
		Capabilities:   []string{driver.CapEdit, driver.CapBackup, driver.CapRoles, driver.CapDatabases, driver.CapStructure},
	}})
	driver.Register(&Driver{info: driver.Info{
		ID: "cockroachdb", Name: "CockroachDB", Family: driver.FamilySQL, Language: "sql",
		DefaultPort: 26257, DefaultUser: "root", Fields: fields,
		SSLModes:       []string{"disable", "require", "verify-full"},
		URIPlaceholder: "postgresql://root@localhost:26257/defaultdb?sslmode=disable",
		Example:        "SHOW DATABASES;",
		Capabilities:   []string{driver.CapEdit, driver.CapDatabases, driver.CapStructure},
	}})
}

type Driver struct{ info driver.Info }

func (d *Driver) Info() driver.Info { return d.info }

func (d *Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	base, err := d.config(cfg)
	if err != nil {
		return nil, err
	}
	open := func(_ context.Context, database string) (*sql.DB, error) {
		c := base.Copy()
		if database != "" {
			c.Database = database
		}
		return stdlib.OpenDB(*c), nil
	}
	db, err := open(ctx, "")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &sqlbase.Engine{DB: db, Dialect: dialect{}, Default: base.Database, Open: open}, nil
}

func (d *Driver) config(cfg driver.Config) (*pgx.ConnConfig, error) {
	dsn := strings.TrimSpace(cfg.URI)
	if dsn == "" {
		port := cfg.Port
		if port == 0 {
			port = d.info.DefaultPort
		}
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		u := url.URL{Scheme: "postgres", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + cfg.Database}
		if cfg.User != "" {
			u.User = url.UserPassword(cfg.User, cfg.Password)
		}
		q := url.Values{}
		if cfg.SSLMode != "" {
			q.Set("sslmode", cfg.SSLMode)
		}
		q.Set("application_name", "Quarry")
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	c, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid connection settings: %w", err)
	}
	if cfg.URI != "" && cfg.Password != "" && c.Password == "" {
		c.Password = cfg.Password
	}
	// Simple protocol lets the editor run multi-statement scripts and lets
	// the server coerce text parameters into each column's type.
	c.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	if c.Database == "" {
		c.Database = c.User
	}
	return c, nil
}

type dialect struct{}

func (dialect) Quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

func (dialect) Use(string) string { return "" }

func (dialect) Paginate(l, o int, ord bool) string { return sqlbase.LimitOffset(l, o, ord) }

func (d dialect) Table(path []string) (string, string) {
	if len(path) != 3 || isServerPath(path) {
		return "", ""
	}
	return path[0], d.Quote(path[1]) + "." + d.Quote(path[2])
}

func (d dialect) Children(ctx context.Context, e *sqlbase.Engine, path []string) ([]driver.Node, error) {
	if isServerPath(path) {
		return d.serverChildren(ctx, e, path)
	}
	switch len(path) {
	case 0:
		names, err := sqlbase.Strings(ctx, e.DB, `SELECT datname FROM pg_database WHERE NOT datistemplate AND datallowconn ORDER BY datname`)
		if err != nil {
			return nil, err
		}
		return append(sqlbase.Databases(names), serverGroups()...), nil
	case 1:
		var out []driver.Node
		err := e.Scoped(ctx, path[0], func(q sqlbase.Querier) error {
			names, err := sqlbase.Strings(ctx, q, `SELECT nspname FROM pg_namespace
				WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema' AND nspname NOT IN ('crdb_internal')
				ORDER BY nspname = 'public' DESC, nspname`)
			out = sqlbase.Schemas(path[0], names)
			return err
		})
		return out, err
	case 2:
		var out []driver.Node
		err := e.Scoped(ctx, path[0], func(q sqlbase.Querier) error {
			pairs, err := sqlbase.Pairs(ctx, q, `SELECT table_name, table_type FROM information_schema.tables
				WHERE table_schema = $1 ORDER BY table_name`, path[1])
			out = sqlbase.Tables(path, pairs)
			return err
		})
		return out, err
	}
	return nil, nil
}
