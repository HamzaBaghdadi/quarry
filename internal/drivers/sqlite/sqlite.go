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
		Capabilities:   []string{driver.CapEdit, driver.CapBackup, driver.CapStructure},
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

// --- Editing -------------------------------------------------------------

func (dialect) Placeholder(int) string { return "?" }

func (dialect) PrimaryKey(ctx context.Context, e *sqlbase.Engine, path []string) ([]string, error) {
	if len(path) != 1 {
		return nil, nil
	}
	return sqlbase.Strings(ctx, e.DB, "SELECT name FROM pragma_table_info(?) WHERE pk > 0 ORDER BY pk", path[0])
}

// --- Backup --------------------------------------------------------------

var literals = sqlbase.LiteralStyle{
	TrueFalse:  [2]string{"1", "0"},
	Bytes:      sqlbase.HexBytes,
	TimeLayout: "2006-01-02 15:04:05.999999999-07:00",
}

func (dialect) Literal(v any) string { return literals.Literal(v) }

func (d dialect) Plan(ctx context.Context, q sqlbase.Querier, _ string) (*sqlbase.DumpPlan, error) {
	plan := &sqlbase.DumpPlan{
		Header: []string{"PRAGMA foreign_keys = OFF", "BEGIN"},
		Footer: []string{"COMMIT", "PRAGMA foreign_keys = ON"},
	}
	tables, err := sqlbase.Pairs(ctx, q, `SELECT name, sql FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND sql IS NOT NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	for _, t := range tables {
		plan.Pre = append(plan.Pre, "DROP TABLE IF EXISTS "+d.Quote(t[0]), t[1])
		plan.Tables = append(plan.Tables, sqlbase.DumpTable{Name: t[0], Source: d.Quote(t[0]), Target: d.Quote(t[0])})
	}
	others, err := sqlbase.Pairs(ctx, q, `SELECT type || ' ' || name, sql FROM sqlite_master
		WHERE type IN ('index','trigger','view') AND sql IS NOT NULL AND name NOT LIKE 'sqlite_%'
		ORDER BY type = 'view' DESC, type`)
	if err != nil {
		return nil, err
	}
	for _, o := range others {
		kind, name, _ := strings.Cut(o[0], " ")
		plan.Post = append(plan.Post, "DROP "+strings.ToUpper(kind)+" IF EXISTS "+d.Quote(name), o[1])
	}
	return plan, nil
}

func (dialect) SearchExpr(col, term string) string {
	return "CAST(" + col + " AS TEXT) LIKE " + sqlbase.QuoteLike(term, false)
}
