package sqlbase

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// DumpTable describes one table in a logical backup.
type DumpTable struct {
	Name    string   // for progress messages
	Source  string   // qualified name to SELECT from
	Target  string   // name to INSERT into (usually unqualified, so dumps restore anywhere)
	Columns []string // columns to copy (nil = all); excludes generated columns
	// InsertPrefix goes between the table and VALUES, e.g. Postgres'
	// "OVERRIDING SYSTEM VALUE" for GENERATED ALWAYS identity columns.
	InsertPrefix string
}

// DumpPlan is everything a dialect contributes to a backup.
type DumpPlan struct {
	Header []string    // session setup, BEGIN, …
	Pre    []string    // DROP/CREATE before data
	Tables []DumpTable // data, in order
	Post   []string    // constraints, indexes, views, sequence values
	Footer []string    // COMMIT, …
	// Snapshot, if set, wraps reads in a consistent transaction.
	Snapshot *sql.TxOptions
}

// Dumper is implemented by dialects that support logical backups.
type Dumper interface {
	Plan(ctx context.Context, q Querier, scope string) (*DumpPlan, error)
	// Literal renders a scanned value as SQL.
	Literal(v any) string
}

// SplitOptions tune the restore-side statement splitter.
type SplitOptions struct {
	DollarQuotes bool // Postgres $tag$ … $tag$
	Backslashes  bool // MySQL: backslash escapes inside strings
}

// Splitter is optionally implemented by Dumper dialects.
type Splitter interface{ SplitOptions() SplitOptions }

const insertBatch = 200

func (e *Engine) BackupFormat() string { return "sql" }

// pinned returns a single connection pointed at scope.
func (e *Engine) pinned(ctx context.Context, scope string) (*sql.Conn, error) {
	pool := e.DB
	if scope != "" && !strings.EqualFold(scope, e.Default) && e.Open != nil {
		p, err := e.pool(ctx, scope)
		if err != nil {
			return nil, err
		}
		pool = p
	}
	conn, err := pool.Conn(ctx)
	if err != nil {
		return nil, err
	}
	if scope != "" && pool == e.DB {
		if use := e.Dialect.Use(scope); use != "" {
			if _, err := conn.ExecContext(ctx, use); err != nil {
				conn.Close()
				return nil, fmt.Errorf("switch to %s: %w", scope, err)
			}
		}
	}
	return conn, nil
}

func (e *Engine) Backup(ctx context.Context, scope string, w io.Writer, p driver.Progress) (driver.BackupStats, error) {
	var st driver.BackupStats
	d, ok := e.Dialect.(Dumper)
	if !ok {
		return st, errors.New("backups aren't supported for this database yet")
	}
	conn, err := e.pinned(ctx, scope)
	if err != nil {
		return st, err
	}
	defer conn.Close()

	var q Querier = conn
	plan, err := d.Plan(ctx, q, scope)
	if err != nil {
		return st, err
	}
	if plan.Snapshot != nil {
		tx, err := conn.BeginTx(ctx, plan.Snapshot)
		if err != nil {
			return st, err
		}
		defer tx.Rollback() //nolint:errcheck
		q = tx
	}

	bw := bufio.NewWriterSize(w, 1<<16)
	put := func(stmts ...string) {
		for _, s := range stmts {
			bw.WriteString(s)
			bw.WriteString(";\n")
		}
	}
	fmt.Fprintf(bw, "-- Quarry backup of %q\n-- Created %s\n-- Restore with Quarry (Restore…) or any SQL client.\n\n", scope, time.Now().Format(time.RFC3339))
	put(plan.Header...)
	bw.WriteString("\n")
	put(plan.Pre...)

	for i, t := range plan.Tables {
		p(fmt.Sprintf("Table %d/%d · %s", i+1, len(plan.Tables), t.Name))
		n, err := e.dumpTable(ctx, q, d, t, bw)
		if err != nil {
			return st, fmt.Errorf("%s: %w", t.Name, err)
		}
		st.Rows += n
		st.Objects++
	}
	bw.WriteString("\n")
	put(plan.Post...)
	put(plan.Footer...)
	return st, bw.Flush()
}

func (e *Engine) dumpTable(ctx context.Context, q Querier, d Dumper, t DumpTable, w *bufio.Writer) (int64, error) {
	sel := "*"
	if len(t.Columns) > 0 {
		quoted := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			quoted[i] = e.Dialect.Quote(c)
		}
		sel = strings.Join(quoted, ", ")
	}
	rows, err := q.QueryContext(ctx, "SELECT "+sel+" FROM "+t.Source)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	names := make([]string, len(cols))
	for i, c := range cols {
		names[i] = e.Dialect.Quote(c)
	}
	head := "INSERT INTO " + t.Target + " (" + strings.Join(names, ", ") + ") "
	if t.InsertPrefix != "" {
		head += t.InsertPrefix + " "
	}
	head += "VALUES\n"

	fmt.Fprintf(w, "\n-- Data for %s\n", t.Name)
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	var n int64
	inBatch := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return n, err
		}
		if inBatch == 0 {
			w.WriteString(head)
		} else {
			w.WriteString(",\n")
		}
		w.WriteString("(")
		for i, v := range vals {
			if i > 0 {
				w.WriteString(", ")
			}
			w.WriteString(d.Literal(v))
		}
		w.WriteString(")")
		n++
		inBatch++
		if inBatch == insertBatch {
			w.WriteString(";\n")
			inBatch = 0
		}
	}
	if inBatch > 0 {
		w.WriteString(";\n")
	}
	return n, rows.Err()
}

func (e *Engine) Restore(ctx context.Context, scope string, r io.Reader, p driver.Progress) (driver.BackupStats, error) {
	var st driver.BackupStats
	conn, err := e.pinned(ctx, scope)
	if err != nil {
		return st, err
	}
	defer conn.Close()

	var opts SplitOptions
	if s, ok := e.Dialect.(Splitter); ok {
		opts = s.SplitOptions()
	}
	failed := false
	defer func() {
		if failed { // leave no half-open transaction on the pooled connection
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	err = SplitStatements(bufio.NewReaderSize(r, 1<<16), opts, func(stmt string) error {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			failed = true
			preview := stmt
			if len(preview) > 160 {
				preview = preview[:160] + "…"
			}
			return fmt.Errorf("statement %d failed: %w\n\n%s", st.Statements+1, err, preview)
		}
		st.Statements++
		if strings.HasPrefix(stmt, "INSERT") {
			st.Rows += int64(strings.Count(stmt, "),\n(") + 1)
		} else if strings.HasPrefix(stmt, "CREATE TABLE") {
			st.Objects++
		}
		if st.Statements%50 == 0 {
			p(fmt.Sprintf("%d statements · %d rows", st.Statements, st.Rows))
		}
		return nil
	})
	return st, err
}

// SplitStatements reads SQL and calls fn for each complete statement,
// honouring quotes, comments and (optionally) dollar quoting.
func SplitStatements(r *bufio.Reader, o SplitOptions, fn func(string) error) error {
	var b strings.Builder
	var quote rune   // ', ", `
	var dollar string // active $tag$
	line, block := false, false
	var prev rune

	flush := func() error {
		s := strings.TrimSpace(b.String())
		b.Reset()
		if s == "" || onlyComments(s) {
			return nil
		}
		return fn(s)
	}

	for {
		c, _, err := r.ReadRune()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch {
		case line:
			if c == '\n' {
				line = false
				b.WriteRune('\n')
			}
		case block:
			if prev == '*' && c == '/' {
				block = false
				b.WriteRune(' ')
				c = 0
			}
		case dollar != "":
			b.WriteRune(c)
			if c == '$' && strings.HasSuffix(b.String(), dollar) && len(b.String()) > len(dollar) {
				dollar = ""
			}
			prev = c
			continue
		case quote != 0:
			b.WriteRune(c)
			if o.Backslashes && c == '\\' {
				if n, _, err := r.ReadRune(); err == nil {
					b.WriteRune(n)
				}
				prev = 0
				continue
			}
			if c == quote {
				quote = 0
			}
			prev = c
			continue
		case c == '-' && prev == '-':
			// Drop the first '-' already written.
			s := b.String()
			b.Reset()
			b.WriteString(s[:len(s)-1])
			line = true
		case c == '*' && prev == '/':
			s := b.String()
			b.Reset()
			b.WriteString(s[:len(s)-1])
			block = true
		case c == '\'' || c == '"' || c == '`':
			quote = c
			b.WriteRune(c)
		case c == '$' && o.DollarQuotes:
			b.WriteRune(c)
			if tag, ok := readDollarTag(r); ok {
				b.WriteString(tag)
				dollar = "$" + tag
			}
		case c == ';':
			if err := flush(); err != nil {
				return err
			}
		default:
			b.WriteRune(c)
		}
		if line || block {
			prev = c
			continue
		}
		prev = c
	}
	return flush()
}

// readDollarTag consumes "tag$" after a '$' if it forms a dollar-quote opener.
func readDollarTag(r *bufio.Reader) (string, bool) {
	peek, _ := r.Peek(64)
	for i, ch := range string(peek) {
		if ch == '$' {
			tag := string(peek[:i+1])
			_, _ = r.Discard(i + 1)
			return tag, true
		}
		if !(ch == '_' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || i > 0 && ch >= '0' && ch <= '9') {
			return "", false
		}
	}
	return "", false
}

func onlyComments(s string) bool {
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "--") {
			return false
		}
	}
	return true
}
