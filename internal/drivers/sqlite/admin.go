package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

func (d dialect) Structure(ctx context.Context, e *sqlbase.Engine, path []string) (*driver.TableStructure, error) {
	if len(path) != 1 {
		return nil, errors.New("pick a table or view")
	}
	name := path[0]
	st := &driver.TableStructure{RowEstimate: -1, Kind: "table"}

	var typ string
	var ddl sql.NullString
	if err := e.DB.QueryRowContext(ctx, `SELECT type, sql FROM sqlite_master WHERE name = ? AND type IN ('table','view')`, name).Scan(&typ, &ddl); err != nil {
		return nil, err
	}
	st.Kind = typ
	var b strings.Builder
	b.WriteString(strings.TrimSpace(ddl.String) + ";")

	cols, err := e.DB.QueryContext(ctx, `SELECT name, type, "notnull" = 0, COALESCE(dflt_value, ''), pk > 0 FROM pragma_table_info(?) ORDER BY cid`, name)
	if err != nil {
		return nil, err
	}
	for cols.Next() {
		var c driver.StructColumn
		if err := cols.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &c.PrimaryKey); err != nil {
			cols.Close()
			return nil, err
		}
		if c.PrimaryKey {
			c.Nullable = false
		}
		st.Columns = append(st.Columns, c)
	}
	cols.Close()

	if typ == "table" {
		// Indexes (with their columns and definitions).
		idx, err := sqlbase.Pairs(ctx, e.DB, `SELECT il.name, CASE WHEN il."unique" THEN 'unique index' ELSE 'index' END
			FROM pragma_index_list(?) il ORDER BY il.name`, name)
		if err != nil {
			return nil, err
		}
		for _, ix := range idx {
			var def sql.NullString
			_ = e.DB.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ?`, ix[0]).Scan(&def)
			colNames, _ := sqlbase.Strings(ctx, e.DB, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, ix[0])
			definition := def.String
			if definition == "" {
				definition = "(" + strings.Join(colNames, ", ") + ") — automatic"
			} else {
				b.WriteString("\n" + definition + ";")
			}
			st.Indexes = append(st.Indexes, driver.StructItem{Name: ix[0], Kind: ix[1], Definition: definition})
		}

		// Foreign keys.
		fks, err := e.DB.QueryContext(ctx, `SELECT id, "table", group_concat("from", ', '), group_concat("to", ', ')
			FROM pragma_foreign_key_list(?) GROUP BY id ORDER BY id`, name)
		if err != nil {
			return nil, err
		}
		for fks.Next() {
			var id int
			var ref, from, to string
			if err := fks.Scan(&id, &ref, &from, &to); err != nil {
				fks.Close()
				return nil, err
			}
			st.Constraints = append(st.Constraints, driver.StructItem{
				Name: fmt.Sprintf("fk_%d", id), Kind: "foreign key",
				Definition: fmt.Sprintf("FOREIGN KEY (%s) REFERENCES %s (%s)", from, d.Quote(ref), to),
			})
		}
		fks.Close()

		var n int64
		if err := e.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+d.Quote(name)).Scan(&n); err == nil {
			st.RowEstimate = n
		}
	}
	st.DDL = b.String()
	return st, nil
}
