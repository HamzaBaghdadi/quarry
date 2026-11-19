// Package sqlbase implements driver.Conn once for every database/sql engine.
// Each SQL adapter supplies a small Dialect (Template Method pattern): how to
// quote identifiers, list the explorer tree, paginate and switch databases.
package sqlbase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// Querier is satisfied by *sql.DB and *sql.Conn.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Dialect captures everything engine-specific.
type Dialect interface {
	// Quote quotes an identifier.
	Quote(ident string) string
	// Children lists explorer nodes under path using the given engine.
	Children(ctx context.Context, e *Engine, path []string) ([]driver.Node, error)
	// Table resolves a browsable path to (scope database, qualified table name).
	Table(path []string) (scope string, ref string)
	// Paginate returns the trailing clause for LIMIT/OFFSET.
	Paginate(limit, offset int, ordered bool) string
	// Use returns the statement that switches the session to db ("" = unsupported).
	Use(db string) string
}

// Opener lets a dialect keep one pool per database (Postgres can't switch
// databases on a live session). Return nil to use Use() on a pinned conn.
type Opener func(ctx context.Context, database string) (*sql.DB, error)

// Engine is the shared driver.Conn implementation.
type Engine struct {
	DB      *sql.DB
	Dialect Dialect
	Default string // database the connection was opened on
	Open    Opener

	mu    sync.Mutex
	pools map[string]*sql.DB
}

func (e *Engine) Ping(ctx context.Context) error { return e.DB.PingContext(ctx) }

func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	var errs []error
	for _, p := range e.pools {
		errs = append(errs, p.Close())
	}
	errs = append(errs, e.DB.Close())
	return errors.Join(errs...)
}

func (e *Engine) Children(ctx context.Context, path []string) ([]driver.Node, error) {
	return e.Dialect.Children(ctx, e, path)
}

// Scoped runs fn with a Querier pointed at database scope.
func (e *Engine) Scoped(ctx context.Context, scope string, fn func(q Querier) error) error {
	if scope == "" || strings.EqualFold(scope, e.Default) {
		return fn(e.DB)
	}
	if e.Open != nil {
		pool, err := e.pool(ctx, scope)
		if err != nil {
			return err
		}
		return fn(pool)
	}
	use := e.Dialect.Use(scope)
	if use == "" {
		return fn(e.DB)
	}
	conn, err := e.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, use); err != nil {
		return fmt.Errorf("switch to %s: %w", scope, err)
	}
	return fn(conn)
}

func (e *Engine) pool(ctx context.Context, name string) (*sql.DB, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pools == nil {
		e.pools = map[string]*sql.DB{}
	}
	if p, ok := e.pools[name]; ok {
		return p, nil
	}
	p, err := e.Open(ctx, name)
	if err != nil {
		return nil, err
	}
	p.SetMaxOpenConns(4)
	e.pools[name] = p
	return p, nil
}

func (e *Engine) Browse(ctx context.Context, req driver.BrowseRequest) (*driver.Result, error) {
	t := driver.StartTimer()
	scope, ref := e.Dialect.Table(req.Path)
	if ref == "" {
		return nil, errors.New("this item can't be browsed")
	}
	if req.Limit <= 0 {
		req.Limit = 200
	}
	var b strings.Builder
	b.WriteString("SELECT * FROM ")
	b.WriteString(ref)
	if f := strings.TrimSpace(req.Filter); f != "" {
		b.WriteString(" WHERE ")
		b.WriteString(f)
	}
	ordered := req.OrderBy != ""
	if ordered {
		b.WriteString(" ORDER BY ")
		b.WriteString(e.Dialect.Quote(req.OrderBy))
		if req.Desc {
			b.WriteString(" DESC")
		}
	}
	// Fetch one extra row to know whether there's another page.
	b.WriteString(" ")
	b.WriteString(e.Dialect.Paginate(req.Limit+1, req.Offset, ordered))
	stmt := b.String()

	var res *driver.Result
	err := e.Scoped(ctx, scope, func(q Querier) error {
		rows, err := q.QueryContext(ctx, stmt)
		if err != nil {
			return err
		}
		defer rows.Close()
		res, err = ReadRows(rows, req.Limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	res.Statement = stmt
	return t.Stamp(res), nil
}

// MaxRows caps how many rows an ad-hoc query returns to the UI.
const MaxRows = 5000

func (e *Engine) Execute(ctx context.Context, scope, query string) (*driver.Result, error) {
	t := driver.StartTimer()
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is empty")
	}
	var res *driver.Result
	err := e.Scoped(ctx, scope, func(q Querier) error {
		var err error
		if ReturnsRows(query) {
			var rows *sql.Rows
			rows, err = q.QueryContext(ctx, query)
			if err != nil {
				return err
			}
			defer rows.Close()
			res, err = ReadRows(rows, MaxRows)
			return err
		}
		r, err := q.ExecContext(ctx, query)
		if err != nil {
			return err
		}
		n, _ := r.RowsAffected()
		res = &driver.Result{RowsAffected: n, Message: fmt.Sprintf("%d row(s) affected", n)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t.Stamp(res), nil
}

// ReturnsRows guesses whether a statement produces a result set.
func ReturnsRows(q string) bool {
	q = strings.ToLower(stripLeadingComments(q))
	for _, kw := range []string{"select", "with", "show", "describe", "desc ", "explain", "pragma", "values", "table ", "exec", "call"} {
		if strings.HasPrefix(q, kw) {
			return true
		}
	}
	return strings.Contains(q, " returning ")
}

func stripLeadingComments(q string) string {
	for {
		q = strings.TrimSpace(q)
		switch {
		case strings.HasPrefix(q, "--"):
			if i := strings.IndexByte(q, '\n'); i >= 0 {
				q = q[i+1:]
				continue
			}
			return ""
		case strings.HasPrefix(q, "/*"):
			if i := strings.Index(q, "*/"); i >= 0 {
				q = q[i+2:]
				continue
			}
			return ""
		}
		return q
	}
}

// ReadRows drains rows into a Result, keeping at most limit rows.
func ReadRows(rows *sql.Rows, limit int) (*driver.Result, error) {
	cols, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	res := &driver.Result{Columns: make([]driver.Column, len(cols)), Rows: [][]any{}}
	for i, c := range cols {
		res.Columns[i] = driver.Column{Name: c.Name(), Type: strings.ToLower(c.DatabaseTypeName())}
	}
	for rows.Next() {
		if len(res.Rows) >= limit {
			res.HasMore = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		for i, v := range vals {
			vals[i] = driver.Normalize(v)
		}
		res.Rows = append(res.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return res, nil
}

// Strings runs a query returning one string column per row (handy for trees).
func Strings(ctx context.Context, q Querier, query string, args ...any) ([]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Pairs runs a query returning (name, kind) rows.
func Pairs(ctx context.Context, q Querier, query string, args ...any) ([][2]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]string
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return nil, err
		}
		out = append(out, [2]string{a, b})
	}
	return out, rows.Err()
}

// Tables turns (name, type) pairs into table/view nodes under parent.
func Tables(parent []string, pairs [][2]string) []driver.Node {
	nodes := make([]driver.Node, 0, len(pairs))
	for _, p := range pairs {
		kind := driver.KindTable
		if strings.Contains(strings.ToLower(p[1]), "view") {
			kind = driver.KindView
		}
		nodes = append(nodes, driver.Node{
			Name: p[0], Kind: kind, Browsable: true,
			Path: append(append([]string{}, parent...), p[0]),
		})
	}
	return nodes
}

// Databases turns names into expandable database nodes.
func Databases(names []string) []driver.Node {
	nodes := make([]driver.Node, 0, len(names))
	for _, n := range names {
		nodes = append(nodes, driver.Node{Name: n, Kind: driver.KindDatabase, Path: []string{n}, HasKids: true})
	}
	return nodes
}

// Schemas turns names into expandable schema nodes under db.
func Schemas(db string, names []string) []driver.Node {
	nodes := make([]driver.Node, 0, len(names))
	for _, n := range names {
		nodes = append(nodes, driver.Node{Name: n, Kind: driver.KindSchema, Path: []string{db, n}, HasKids: true})
	}
	return nodes
}

// LimitOffset is the common LIMIT/OFFSET clause.
func LimitOffset(limit, offset int, _ bool) string {
	return fmt.Sprintf("LIMIT %d OFFSET %d", limit, offset)
}
