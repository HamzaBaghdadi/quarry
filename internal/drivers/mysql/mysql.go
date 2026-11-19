// Package mysql adapts MySQL and compatible engines (MariaDB, TiDB, PlanetScale, Vitess).
package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"regexp"
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
		info.Capabilities = []string{driver.CapEdit, driver.CapBackup}
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
	c.ClientFoundRows = true // UPDATE reports matched rows, so edit checks are exact
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

// --- Editing -------------------------------------------------------------

func (dialect) Placeholder(int) string { return "?" }
func (dialect) EmptyInsert() string    { return "() VALUES ()" }

func (dialect) PrimaryKey(ctx context.Context, e *sqlbase.Engine, path []string) ([]string, error) {
	if len(path) != 2 {
		return nil, nil
	}
	return sqlbase.Strings(ctx, e.DB, `SELECT COLUMN_NAME FROM information_schema.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND CONSTRAINT_NAME = 'PRIMARY' ORDER BY ORDINAL_POSITION`, path[0], path[1])
}

// --- Backup --------------------------------------------------------------

var literals = sqlbase.LiteralStyle{
	TrueFalse:   [2]string{"1", "0"},
	Bytes:       sqlbase.HexBytes,
	Backslashes: true,
	TimeLayout:  "2006-01-02 15:04:05.999999",
}

func (dialect) Literal(v any) string               { return literals.Literal(v) }
func (dialect) SplitOptions() sqlbase.SplitOptions { return sqlbase.SplitOptions{Backslashes: true} }

var definer = regexp.MustCompile(`(?i)\s+DEFINER\s*=\s*\S+@\S+`)

func (d dialect) Plan(ctx context.Context, q sqlbase.Querier, scope string) (*sqlbase.DumpPlan, error) {
	if scope == "" {
		return nil, errors.New("pick a database to back up")
	}
	plan := &sqlbase.DumpPlan{
		Header:   []string{"SET NAMES utf8mb4", "SET FOREIGN_KEY_CHECKS = 0", "SET UNIQUE_CHECKS = 0"},
		Footer:   []string{"SET UNIQUE_CHECKS = 1", "SET FOREIGN_KEY_CHECKS = 1"},
		Snapshot: &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true},
	}
	objs, err := sqlbase.Pairs(ctx, q, `SELECT TABLE_NAME, TABLE_TYPE FROM information_schema.TABLES
		WHERE TABLE_SCHEMA = ? ORDER BY TABLE_TYPE = 'VIEW', TABLE_NAME`, scope)
	if err != nil {
		return nil, err
	}
	qualified := func(t string) string { return d.Quote(scope) + "." + d.Quote(t) }
	for _, o := range objs {
		name, isView := o[0], strings.Contains(o[1], "VIEW")
		var ddl string
		var ignored string
		stmt := "SHOW CREATE TABLE " + qualified(name)
		if isView {
			stmt = "SHOW CREATE VIEW " + qualified(name)
		}
		row := q.QueryRowContext(ctx, stmt)
		if isView {
			var cs, coll string
			err = row.Scan(&ignored, &ddl, &cs, &coll)
		} else {
			err = row.Scan(&ignored, &ddl)
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if isView {
			ddl = strings.ReplaceAll(definer.ReplaceAllString(ddl, ""), d.Quote(scope)+".", "")
			plan.Post = append(plan.Post, "DROP VIEW IF EXISTS "+d.Quote(name), ddl)
			continue
		}
		cols, err := sqlbase.Strings(ctx, q, `SELECT COLUMN_NAME FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? AND EXTRA NOT LIKE '%GENERATED%' ORDER BY ORDINAL_POSITION`, scope, name)
		if err != nil {
			return nil, err
		}
		plan.Pre = append(plan.Pre, "DROP TABLE IF EXISTS "+d.Quote(name), ddl)
		plan.Tables = append(plan.Tables, sqlbase.DumpTable{Name: name, Source: qualified(name), Target: d.Quote(name), Columns: cols})
	}
	return plan, nil
}
