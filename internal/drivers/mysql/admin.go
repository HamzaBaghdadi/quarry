package mysql

import (
	"context"
	"errors"
	"strings"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

func (d dialect) CreateDatabase(ctx context.Context, e *sqlbase.Engine, name, _ string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("give the database a name")
	}
	_, err := e.DB.ExecContext(ctx, "CREATE DATABASE "+d.Quote(name))
	return err
}

func (d dialect) DropDatabase(ctx context.Context, e *sqlbase.Engine, name string) error {
	if system[strings.ToLower(name)] {
		return errors.New("system databases can't be dropped")
	}
	_, err := e.DB.ExecContext(ctx, "DROP DATABASE "+d.Quote(name))
	return err
}

func (d dialect) Structure(ctx context.Context, e *sqlbase.Engine, path []string) (*driver.TableStructure, error) {
	if len(path) != 2 {
		return nil, errors.New("pick a table or view")
	}
	db, tbl := path[0], path[1]
	ref := d.Quote(db) + "." + d.Quote(tbl)
	st := &driver.TableStructure{RowEstimate: -1, Kind: "table"}

	var typ string
	var rows int64
	var size string
	err := e.DB.QueryRowContext(ctx, `SELECT TABLE_TYPE, COALESCE(TABLE_ROWS, -1),
			COALESCE(CONCAT(ROUND((DATA_LENGTH + INDEX_LENGTH) / 1024 / 1024, 2), ' MB'), ''), COALESCE(TABLE_COMMENT, '')
		FROM information_schema.TABLES WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?`, db, tbl).Scan(&typ, &rows, &size, &st.Comment)
	if err != nil {
		return nil, err
	}
	st.RowEstimate, st.Size = rows, size
	if strings.Contains(typ, "VIEW") {
		st.Kind = "view"
	}

	cols, err := e.DB.QueryContext(ctx, `SELECT COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE = 'YES', COALESCE(COLUMN_DEFAULT, ''),
			COLUMN_KEY = 'PRI', COALESCE(COLUMN_COMMENT, '')
		FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ? ORDER BY ORDINAL_POSITION`, db, tbl)
	if err != nil {
		return nil, err
	}
	for cols.Next() {
		var c driver.StructColumn
		if err := cols.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &c.PrimaryKey, &c.Comment); err != nil {
			cols.Close()
			return nil, err
		}
		st.Columns = append(st.Columns, c)
	}
	cols.Close()

	if st.Kind == "table" {
		idx, err := e.DB.QueryContext(ctx, `SELECT INDEX_NAME, MIN(NON_UNIQUE),
				GROUP_CONCAT(CONCAT('`+"`"+`', COLUMN_NAME, '`+"`"+`') ORDER BY SEQ_IN_INDEX SEPARATOR ', ')
			FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = ? AND TABLE_NAME = ?
			GROUP BY INDEX_NAME ORDER BY INDEX_NAME = 'PRIMARY' DESC, INDEX_NAME`, db, tbl)
		if err != nil {
			return nil, err
		}
		for idx.Next() {
			var name, cols string
			var nonUnique int
			if err := idx.Scan(&name, &nonUnique, &cols); err != nil {
				idx.Close()
				return nil, err
			}
			kind := "index"
			switch {
			case name == "PRIMARY":
				kind = "primary key"
			case nonUnique == 0:
				kind = "unique index"
			}
			st.Indexes = append(st.Indexes, driver.StructItem{Name: name, Kind: kind, Definition: "(" + cols + ")"})
		}
		idx.Close()

		cons, err := e.DB.QueryContext(ctx, `SELECT tc.CONSTRAINT_NAME, LOWER(tc.CONSTRAINT_TYPE),
				COALESCE(CONCAT('REFERENCES `+"`"+`', MAX(k.REFERENCED_TABLE_NAME), '`+"`"+`'), '')
			FROM information_schema.TABLE_CONSTRAINTS tc
			LEFT JOIN information_schema.KEY_COLUMN_USAGE k
			  ON k.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA AND k.CONSTRAINT_NAME = tc.CONSTRAINT_NAME AND k.TABLE_NAME = tc.TABLE_NAME
			WHERE tc.TABLE_SCHEMA = ? AND tc.TABLE_NAME = ?
			GROUP BY tc.CONSTRAINT_NAME, tc.CONSTRAINT_TYPE ORDER BY tc.CONSTRAINT_TYPE = 'PRIMARY KEY' DESC, tc.CONSTRAINT_NAME`, db, tbl)
		if err != nil {
			return nil, err
		}
		for cons.Next() {
			var it driver.StructItem
			if err := cons.Scan(&it.Name, &it.Kind, &it.Definition); err != nil {
				cons.Close()
				return nil, err
			}
			st.Constraints = append(st.Constraints, it)
		}
		cons.Close()
	}

	stmt := "SHOW CREATE TABLE " + ref
	if st.Kind == "view" {
		stmt = "SHOW CREATE VIEW " + ref
	}
	r, err := e.DB.QueryContext(ctx, stmt)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if r.Next() {
		cols, _ := r.Columns()
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := r.Scan(ptrs...); err != nil {
			return nil, err
		}
		if b, ok := vals[1].([]byte); ok {
			st.DDL = definer.ReplaceAllString(string(b), "") + ";"
		} else if s, ok := vals[1].(string); ok {
			st.DDL = definer.ReplaceAllString(s, "") + ";"
		}
	}
	return st, r.Err()
}
