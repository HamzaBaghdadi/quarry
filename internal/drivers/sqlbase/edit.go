package sqlbase

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// KeyFinder is implemented by dialects that can report a table's primary key.
// Dialects without it make every table read-only.
type KeyFinder interface {
	PrimaryKey(ctx context.Context, e *Engine, path []string) ([]string, error)
	// Placeholder returns the n-th (1-based) bind parameter: $1, ?, @p1.
	Placeholder(n int) string
}

// EmptyInserter overrides the statement used to insert a row of defaults.
type EmptyInserter interface{ EmptyInsert() string }

type txStarter interface {
	BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error)
}

func (e *Engine) Keys(ctx context.Context, path []string) ([]string, error) {
	kf, ok := e.Dialect.(KeyFinder)
	if !ok {
		return nil, nil
	}
	return kf.PrimaryKey(ctx, e, path)
}

// Apply writes a change set inside one transaction.
func (e *Engine) Apply(ctx context.Context, cs driver.ChangeSet) (*driver.Result, error) {
	t := driver.StartTimer()
	kf, ok := e.Dialect.(KeyFinder)
	if !ok {
		return nil, errors.New("editing isn't supported for this database")
	}
	scope, ref := e.Dialect.Table(cs.Path)
	if ref == "" {
		return nil, errors.New("this item can't be edited")
	}
	q := e.Dialect.Quote
	var affected int64

	err := e.Scoped(ctx, scope, func(qr Querier) error {
		ts, ok := qr.(txStarter)
		if !ok {
			return errors.New("transactions unavailable")
		}
		tx, err := ts.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback() //nolint:errcheck // no-op after commit

		exec := func(stmt string, args []any, mustBeOne bool, what string) error {
			r, err := tx.ExecContext(ctx, stmt, args...)
			if err != nil {
				return fmt.Errorf("%s: %w", what, err)
			}
			n, _ := r.RowsAffected()
			if mustBeOne && n != 1 {
				return fmt.Errorf("%s matched %d rows instead of 1 — the row may have changed or the key isn't unique. Nothing was saved", what, n)
			}
			affected += n
			return nil
		}

		for i, d := range cs.Deletes {
			where, args := whereKey(q, kf, d, nil)
			if err := exec("DELETE FROM "+ref+" WHERE "+where, args, true, fmt.Sprintf("delete #%d", i+1)); err != nil {
				return err
			}
		}
		for i, u := range cs.Updates {
			if len(u.Values) == 0 {
				continue
			}
			cols := sortedKeys(u.Values)
			sets := make([]string, len(cols))
			args := make([]any, 0, len(cols)+len(u.Key))
			for j, c := range cols {
				args = append(args, arg(u.Values[c]))
				sets[j] = q(c) + " = " + kf.Placeholder(len(args))
			}
			where, args := whereKey(q, kf, u.Key, args)
			stmt := "UPDATE " + ref + " SET " + strings.Join(sets, ", ") + " WHERE " + where
			if err := exec(stmt, args, true, fmt.Sprintf("update #%d", i+1)); err != nil {
				return err
			}
		}
		for i, row := range cs.Inserts {
			cols := sortedKeys(row)
			var stmt string
			args := make([]any, 0, len(cols))
			if len(cols) == 0 {
				empty := "DEFAULT VALUES"
				if ei, ok := e.Dialect.(EmptyInserter); ok {
					empty = ei.EmptyInsert()
				}
				stmt = "INSERT INTO " + ref + " " + empty
			} else {
				names := make([]string, len(cols))
				ph := make([]string, len(cols))
				for j, c := range cols {
					names[j] = q(c)
					args = append(args, arg(row[c]))
					ph[j] = kf.Placeholder(len(args))
				}
				stmt = "INSERT INTO " + ref + " (" + strings.Join(names, ", ") + ") VALUES (" + strings.Join(ph, ", ") + ")"
			}
			if err := exec(stmt, args, false, fmt.Sprintf("insert #%d", i+1)); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
	if err != nil {
		return nil, err
	}
	return t.Stamp(&driver.Result{RowsAffected: affected, Message: summary(cs)}), nil
}

func whereKey(q func(string) string, kf KeyFinder, key map[string]any, args []any) (string, []any) {
	cols := sortedKeys(key)
	parts := make([]string, len(cols))
	for i, c := range cols {
		if key[c] == nil {
			parts[i] = q(c) + " IS NULL"
			continue
		}
		args = append(args, arg(key[c]))
		parts[i] = q(c) + " = " + kf.Placeholder(len(args))
	}
	if len(parts) == 0 {
		return "1 = 0", args // never match: refuse key-less writes
	}
	return strings.Join(parts, " AND "), args
}

// arg turns a JSON value from the UI into a driver argument.
func arg(v any) any {
	if f, ok := v.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return int64(f)
	}
	return v
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func summary(cs driver.ChangeSet) string {
	var parts []string
	if n := len(cs.Updates); n > 0 {
		parts = append(parts, fmt.Sprintf("%d updated", n))
	}
	if n := len(cs.Inserts); n > 0 {
		parts = append(parts, fmt.Sprintf("%d inserted", n))
	}
	if n := len(cs.Deletes); n > 0 {
		parts = append(parts, fmt.Sprintf("%d deleted", n))
	}
	if len(parts) == 0 {
		return "No changes"
	}
	return "Saved: " + strings.Join(parts, ", ")
}
