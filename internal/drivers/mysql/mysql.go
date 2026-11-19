// Package mysql adapts MySQL and compatible engines (MariaDB, TiDB, PlanetScale, Vitess).
package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

var fields = []driver.Field{driver.FieldHost, driver.FieldPort, driver.FieldUser, driver.FieldPassword, driver.FieldDatabase, driver.FieldSSL, driver.FieldURI}

func init() {
	for _, info := range []driver.Info{
		{ID: "mysql", Name: "MySQL", Example: "SELECT NOW(), VERSION();"},
		{ID: "mariadb", Name: "MariaDB", Example: "SELECT NOW(), VERSION();"},
		{ID: "tidb", Name: "TiDB", Example: "SELECT tidb_version();", DefaultPort: 4000},
	} {
		info.Family, info.Language, info.Fields = driver.FamilySQL, "sql", fields
		if info.DefaultPort == 0 {
			info.DefaultPort = 3306
		}
		info.DefaultUser = "root"
		info.SSLModes = []string{"false", "preferred", "true", "skip-verify"}
		info.URIPlaceholder = "user:pass@tcp(localhost:3306)/db?parseTime=true"
		driver.Register(&Driver{info: info})
	}
}

type Driver struct{ info driver.Info }

func (d *Driver) Info() driver.Info { return d.info }

func (d *Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	var c *gomysql.Config
	if dsn := strings.TrimSpace(cfg.URI); dsn != "" {
		dsn = strings.TrimPrefix(strings.TrimPrefix(dsn, "mysql://"), "mariadb://")
		parsed, err := gomysql.ParseDSN(dsn)
		if err != nil {
			return nil, fmt.Errorf("invalid DSN: %w", err)
		}
		c = parsed
	} else {
		c = gomysql.NewConfig()
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = d.info.DefaultPort
		}
		c.Net, c.Addr = "tcp", net.JoinHostPort(host, strconv.Itoa(port))
		c.User, c.Passwd, c.DBName = cfg.User, cfg.Password, cfg.Database
		if cfg.SSLMode != "" && cfg.SSLMode != "false" {
			c.TLSConfig = cfg.SSLMode
		}
	}
	c.ParseTime = true
	c.Loc = time.UTC
	c.MultiStatements = false
	c.InterpolateParams = true
	connector, err := gomysql.NewConnector(c)
	if err != nil {
		return nil, err
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(8)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &sqlbase.Engine{DB: db, Dialect: dialect{}, Default: c.DBName}, nil
}

type dialect struct{}

func (dialect) Quote(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

func (d dialect) Use(db string) string { return "USE " + d.Quote(db) }

func (dialect) Paginate(l, o int, ord bool) string { return sqlbase.LimitOffset(l, o, ord) }

func (d dialect) Table(path []string) (string, string) {
	if len(path) != 2 {
		return "", ""
	}
	return "", d.Quote(path[0]) + "." + d.Quote(path[1])
}

var system = map[string]bool{"information_schema": true, "performance_schema": true, "mysql": true, "sys": true, "metrics_schema": true}

func (dialect) Children(ctx context.Context, e *sqlbase.Engine, path []string) ([]driver.Node, error) {
	switch len(path) {
	case 0:
		names, err := sqlbase.Strings(ctx, e.DB, "SELECT schema_name FROM information_schema.schemata ORDER BY schema_name")
		if err != nil {
			return nil, err
		}
		user, sys := names[:0:0], []string{}
		for _, n := range names {
			if system[strings.ToLower(n)] {
				sys = append(sys, n)
			} else {
				user = append(user, n)
			}
		}
		return sqlbase.Databases(append(user, sys...)), nil
	case 1:
		pairs, err := sqlbase.Pairs(ctx, e.DB, `SELECT table_name, table_type FROM information_schema.tables
			WHERE table_schema = ? ORDER BY table_name`, path[0])
		if err != nil {
			return nil, err
		}
		return sqlbase.Tables(path, pairs), nil
	}
	return nil, nil
}
