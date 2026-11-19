// Package clickhouse adapts ClickHouse over its native protocol.
package clickhouse

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"

	ch "github.com/ClickHouse/clickhouse-go/v2"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Info() driver.Info {
	return driver.Info{
		ID: "clickhouse", Name: "ClickHouse", Family: driver.FamilySQL, Language: "sql",
		DefaultPort: 9000, DefaultUser: "default",
		Fields:         []driver.Field{driver.FieldHost, driver.FieldPort, driver.FieldUser, driver.FieldPassword, driver.FieldDatabase, driver.FieldSSL, driver.FieldURI},
		SSLModes:       []string{"false", "true"},
		URIPlaceholder: "clickhouse://default:@localhost:9000/default",
		Example:        "SELECT version(), uptime();",
	}
}

func (Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	var opts *ch.Options
	if dsn := strings.TrimSpace(cfg.URI); dsn != "" {
		o, err := ch.ParseDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("invalid DSN: %w", err)
		}
		opts = o
	} else {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 9000
		}
		opts = &ch.Options{
			Addr: []string{net.JoinHostPort(host, strconv.Itoa(port))},
			Auth: ch.Auth{Database: cfg.Database, Username: cfg.User, Password: cfg.Password},
		}
		if cfg.SSLMode == "true" {
			opts.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
		}
	}
	db := ch.OpenDB(opts)
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	database := opts.Auth.Database
	return &sqlbase.Engine{DB: db, Dialect: dialect{}, Default: database}, nil
}

type dialect struct{}

func (dialect) Quote(s string) string { return "`" + strings.ReplaceAll(s, "`", "\\`") + "`" }

func (dialect) Use(string) string { return "" } // tables are always fully qualified

func (dialect) Paginate(l, o int, ord bool) string { return sqlbase.LimitOffset(l, o, ord) }

func (d dialect) Table(path []string) (string, string) {
	if len(path) != 2 {
		return "", ""
	}
	return "", d.Quote(path[0]) + "." + d.Quote(path[1])
}

func (dialect) Children(ctx context.Context, e *sqlbase.Engine, path []string) ([]driver.Node, error) {
	switch len(path) {
	case 0:
		names, err := sqlbase.Strings(ctx, e.DB, "SELECT name FROM system.databases ORDER BY name IN ('system','INFORMATION_SCHEMA','information_schema'), name")
		if err != nil {
			return nil, err
		}
		return sqlbase.Databases(names), nil
	case 1:
		pairs, err := sqlbase.Pairs(ctx, e.DB, "SELECT name, engine FROM system.tables WHERE database = ? ORDER BY name", path[0])
		if err != nil {
			return nil, err
		}
		return sqlbase.Tables(path, pairs), nil
	}
	return nil, nil
}


func (dialect) SearchExpr(col, term string) string {
	return "toString(" + col + ") ILIKE " + sqlbase.QuoteLike(term, true)
}
