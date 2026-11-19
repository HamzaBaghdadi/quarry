// Package mssql adapts Microsoft SQL Server and Azure SQL.
package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	_ "github.com/microsoft/go-mssqldb"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Info() driver.Info {
	return driver.Info{
		ID: "mssql", Name: "SQL Server", Family: driver.FamilySQL, Language: "sql",
		DefaultPort: 1433, DefaultUser: "sa",
		Fields:         []driver.Field{driver.FieldHost, driver.FieldPort, driver.FieldUser, driver.FieldPassword, driver.FieldDatabase, driver.FieldSSL, driver.FieldURI},
		SSLModes:       []string{"false", "true", "strict", "disable"},
		URIPlaceholder: "sqlserver://sa:pass@localhost:1433?database=master",
		Example:        "SELECT @@VERSION;",
		Capabilities:   []string{driver.CapEdit},
	}
}

func (d Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	dsn := strings.TrimSpace(cfg.URI)
	database := cfg.Database
	if dsn == "" {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 1433
		}
		q := url.Values{}
		if database != "" {
			q.Set("database", database)
		}
		enc := cfg.SSLMode
		if enc == "" {
			enc = "false"
		}
		q.Set("encrypt", enc)
		q.Set("TrustServerCertificate", "true")
		q.Set("app name", "Quarry")
		u := url.URL{Scheme: "sqlserver", User: url.UserPassword(cfg.User, cfg.Password),
			Host: net.JoinHostPort(host, strconv.Itoa(port)), RawQuery: q.Encode()}
		dsn = u.String()
	}
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid connection settings: %w", err)
	}
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if database == "" {
		_ = db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&database)
	}
	return &sqlbase.Engine{DB: db, Dialect: dialect{}, Default: database}, nil
}

type dialect struct{}

func (dialect) Quote(s string) string { return "[" + strings.ReplaceAll(s, "]", "]]") + "]" }

func (d dialect) Use(db string) string { return "USE " + d.Quote(db) }

func (dialect) Paginate(l, o int, ordered bool) string {
	order := ""
	if !ordered {
		order = "ORDER BY (SELECT NULL) "
	}
	return fmt.Sprintf("%sOFFSET %d ROWS FETCH NEXT %d ROWS ONLY", order, o, l)
}

func (d dialect) Table(path []string) (string, string) {
	if len(path) != 3 {
		return "", ""
	}
	return "", d.Quote(path[0]) + "." + d.Quote(path[1]) + "." + d.Quote(path[2])
}

func (d dialect) Children(ctx context.Context, e *sqlbase.Engine, path []string) ([]driver.Node, error) {
	switch len(path) {
	case 0:
		names, err := sqlbase.Strings(ctx, e.DB, "SELECT name FROM sys.databases WHERE state = 0 ORDER BY database_id")
		if err != nil {
			return nil, err
		}
		return sqlbase.Databases(names), nil
	case 1:
		db := d.Quote(path[0])
		names, err := sqlbase.Strings(ctx, e.DB, "SELECT DISTINCT TABLE_SCHEMA FROM "+db+".INFORMATION_SCHEMA.TABLES ORDER BY TABLE_SCHEMA")
		if err != nil {
			return nil, err
		}
		return sqlbase.Schemas(path[0], names), nil
	case 2:
		db := d.Quote(path[0])
		pairs, err := sqlbase.Pairs(ctx, e.DB, "SELECT TABLE_NAME, TABLE_TYPE FROM "+db+
			".INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = @p1 ORDER BY TABLE_NAME", path[1])
		if err != nil {
			return nil, err
		}
		return sqlbase.Tables(path, pairs), nil
	}
	return nil, nil
}

// --- Editing -------------------------------------------------------------

func (dialect) Placeholder(n int) string { return "@p" + strconv.Itoa(n) }

func (d dialect) PrimaryKey(ctx context.Context, e *sqlbase.Engine, path []string) ([]string, error) {
	if len(path) != 3 {
		return nil, nil
	}
	db := d.Quote(path[0])
	return sqlbase.Strings(ctx, e.DB, `SELECT kcu.COLUMN_NAME
		FROM `+db+`.INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN `+db+`.INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA AND tc.TABLE_NAME = kcu.TABLE_NAME
		WHERE tc.CONSTRAINT_TYPE = 'PRIMARY KEY' AND tc.TABLE_SCHEMA = @p1 AND tc.TABLE_NAME = @p2
		ORDER BY kcu.ORDINAL_POSITION`, path[1], path[2])
}

func (dialect) SearchExpr(col, term string) string {
	return "TRY_CAST(" + col + " AS NVARCHAR(4000)) LIKE N" + sqlbase.QuoteLike(term, false)
}
