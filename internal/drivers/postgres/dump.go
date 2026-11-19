package postgres

import (
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

// --- Editing -------------------------------------------------------------

func (dialect) Placeholder(n int) string { return "$" + strconv.Itoa(n) }

func (d dialect) PrimaryKey(ctx context.Context, e *sqlbase.Engine, path []string) ([]string, error) {
	if len(path) != 3 {
		return nil, nil
	}
	var keys []string
	err := e.Scoped(ctx, path[0], func(q sqlbase.Querier) error {
		var err error
		keys, err = sqlbase.Strings(ctx, q, `SELECT a.attname FROM pg_index i
			JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
			WHERE i.indrelid = to_regclass($1) AND i.indisprimary
			ORDER BY array_position(i.indkey::int2[], a.attnum)`, d.Quote(path[1])+"."+d.Quote(path[2]))
		return err
	})
	return keys, err
}

// --- Backup --------------------------------------------------------------

var literals = sqlbase.LiteralStyle{
	TrueFalse:  [2]string{"true", "false"},
	Bytes:      func(b []byte) string { return `'\x` + hex.EncodeToString(b) + `'` },
	TimeLayout: "2006-01-02 15:04:05.999999999Z07:00",
	QuoteNaN:   true,
}

func (dialect) Literal(v any) string               { return literals.Literal(v) }
func (dialect) SplitOptions() sqlbase.SplitOptions { return sqlbase.SplitOptions{DollarQuotes: true} }

func (d dialect) Plan(ctx context.Context, q sqlbase.Querier, _ string) (*sqlbase.DumpPlan, error) {
	plan := &sqlbase.DumpPlan{
		Header:   []string{"SET client_encoding = 'UTF8'", "SET standard_conforming_strings = on", "BEGIN"},
		Footer:   []string{"COMMIT"},
		Snapshot: &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true},
	}
	schemas, err := sqlbase.Strings(ctx, q, `SELECT nspname FROM pg_namespace
		WHERE nspname NOT LIKE 'pg\_%' AND nspname <> 'information_schema' ORDER BY nspname`)
	if err != nil {
		return nil, err
	}
	var drops, creates, sequences []string
	for _, s := range schemas {
		qs := d.Quote(s)
		plan.Pre = append(plan.Pre, "CREATE SCHEMA IF NOT EXISTS "+qs)

		// Enum types.
		enums, err := sqlbase.Pairs(ctx, q, `SELECT t.typname, string_agg(quote_literal(e.enumlabel), ', ' ORDER BY e.enumsortorder)
			FROM pg_type t JOIN pg_enum e ON e.enumtypid = t.oid JOIN pg_namespace n ON n.oid = t.typnamespace
			WHERE n.nspname = $1 GROUP BY t.typname ORDER BY t.typname`, s)
		if err != nil {
			return nil, err
		}
		for _, en := range enums {
			plan.Pre = append(plan.Pre, fmt.Sprintf("DO $$ BEGIN CREATE TYPE %s.%s AS ENUM (%s); EXCEPTION WHEN duplicate_object THEN NULL; END $$", qs, d.Quote(en[0]), en[1]))
		}

		// Free-standing sequences (identity sequences are recreated with their tables).
		seqs, err := d.sequences(ctx, q, s)
		if err != nil {
			return nil, err
		}
		for _, sq := range seqs {
			drops = append(drops, "DROP SEQUENCE IF EXISTS "+sq.ref+" CASCADE")
			creates = append(creates, sq.create)
			if sq.setval != "" {
				sequences = append(sequences, sq.setval)
			}
		}

		// Tables.
		tables, err := sqlbase.Strings(ctx, q, `SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relkind IN ('r','p') AND NOT c.relispartition ORDER BY c.relname`, s)
		if err != nil {
			return nil, err
		}
		for _, t := range tables {
			ref := qs + "." + d.Quote(t)
			drops = append([]string{"DROP TABLE IF EXISTS " + ref + " CASCADE"}, drops...)
			ddl, cols, always, identity, err := d.createTable(ctx, q, ref)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ref, err)
			}
			creates = append(creates, ddl)
			dt := sqlbase.DumpTable{Name: s + "." + t, Source: ref, Target: ref, Columns: cols}
			if always {
				dt.InsertPrefix = "OVERRIDING SYSTEM VALUE"
			}
			plan.Tables = append(plan.Tables, dt)
			for _, col := range identity {
				sequences = append(sequences, fmt.Sprintf(
					"SELECT setval(pg_get_serial_sequence(%s, %s), GREATEST(COALESCE((SELECT max(%s) FROM %s), 0), 1), (SELECT count(*) > 0 FROM %s))",
					literals.Str(ref), literals.Str(col), d.Quote(col), ref, ref))
			}
		}

		// Constraints (FKs last), standalone indexes, views.
		cons, err := sqlbase.Pairs(ctx, q, `SELECT format('%I.%I', n.nspname, cl.relname) || ' ' || quote_ident(c.conname), pg_get_constraintdef(c.oid)
			FROM pg_constraint c JOIN pg_class cl ON cl.oid = c.conrelid JOIN pg_namespace n ON n.oid = cl.relnamespace
			WHERE n.nspname = $1 AND c.contype IN ('p','u','c','f','x') AND c.conislocal
			ORDER BY c.contype = 'f', cl.relname, c.conname`, s)
		if err != nil {
			return nil, err
		}
		for _, c := range cons {
			tbl, name, _ := strings.Cut(c[0], " ")
			plan.Post = append(plan.Post, "ALTER TABLE "+tbl+" ADD CONSTRAINT "+name+" "+c[1])
		}
		idx, err := sqlbase.Strings(ctx, q, `SELECT pg_get_indexdef(i.indexrelid) FROM pg_index i
			JOIN pg_class c ON c.oid = i.indexrelid JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND NOT EXISTS (SELECT 1 FROM pg_constraint k WHERE k.conindid = i.indexrelid)`, s)
		if err != nil {
			return nil, err
		}
		plan.Post = append(plan.Post, idx...)
		views, err := sqlbase.Pairs(ctx, q, `SELECT viewname, definition FROM pg_views WHERE schemaname = $1 ORDER BY viewname`, s)
		if err != nil {
			return nil, err
		}
		for _, v := range views {
			plan.Post = append(plan.Post, "CREATE OR REPLACE VIEW "+qs+"."+d.Quote(v[0])+" AS "+strings.TrimSuffix(strings.TrimSpace(v[1]), ";"))
		}
	}
	plan.Pre = append(append(plan.Pre, drops...), creates...)
	plan.Post = append(sequences, plan.Post...)
	return plan, nil
}

type sequence struct{ ref, create, setval string }

func (d dialect) sequences(ctx context.Context, q sqlbase.Querier, schema string) ([]sequence, error) {
	rows, err := q.QueryContext(ctx, `SELECT c.relname, s.seqstart, s.seqincrement, s.seqmin, s.seqmax, s.seqcycle, ps.last_value
		FROM pg_class c
		JOIN pg_sequence s ON s.seqrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		LEFT JOIN pg_sequences ps ON ps.schemaname = n.nspname AND ps.sequencename = c.relname
		WHERE n.nspname = $1 AND c.relkind = 'S'
		  AND NOT EXISTS (SELECT 1 FROM pg_depend dp WHERE dp.objid = c.oid AND dp.deptype = 'i')
		ORDER BY c.relname`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sequence
	for rows.Next() {
		var name string
		var start, inc, min, max int64
		var cycle bool
		var last sql.NullInt64
		if err := rows.Scan(&name, &start, &inc, &min, &max, &cycle, &last); err != nil {
			return nil, err
		}
		ref := d.Quote(schema) + "." + d.Quote(name)
		cyc := "NO CYCLE"
		if cycle {
			cyc = "CYCLE"
		}
		sq := sequence{ref: ref, create: fmt.Sprintf("CREATE SEQUENCE %s START %d INCREMENT %d MINVALUE %d MAXVALUE %d %s", ref, start, inc, min, max, cyc)}
		if last.Valid {
			sq.setval = fmt.Sprintf("SELECT setval(%s, %d, true)", literals.Str(ref), last.Int64)
		}
		out = append(out, sq)
	}
	return out, rows.Err()
}

// createTable rebuilds a table's column definitions (constraints are added
// after the data is loaded).
func (d dialect) createTable(ctx context.Context, q sqlbase.Querier, ref string) (ddl string, dataCols []string, always bool, identity []string, err error) {
	rows, err := q.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), a.attnotnull,
			COALESCE(pg_get_expr(ad.adbin, ad.adrelid), ''), a.attidentity::text, a.attgenerated::text
		FROM pg_attribute a LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
		WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, ref)
	if err != nil {
		return "", nil, false, nil, err
	}
	defer rows.Close()
	var defs []string
	for rows.Next() {
		var name, typ, def, ident, gen string
		var notNull bool
		if err := rows.Scan(&name, &typ, &notNull, &def, &ident, &gen); err != nil {
			return "", nil, false, nil, err
		}
		col := d.Quote(name) + " " + typ
		switch {
		case gen == "s":
			col += " GENERATED ALWAYS AS (" + def + ") STORED"
		case ident == "a":
			col += " GENERATED ALWAYS AS IDENTITY"
			always = true
			identity = append(identity, name)
		case ident == "d":
			col += " GENERATED BY DEFAULT AS IDENTITY"
			identity = append(identity, name)
		case def != "":
			col += " DEFAULT " + def
		}
		if notNull {
			col += " NOT NULL"
		}
		if gen != "s" {
			dataCols = append(dataCols, name)
		}
		defs = append(defs, "  "+col)
	}
	return "CREATE TABLE " + ref + " (\n" + strings.Join(defs, ",\n") + "\n)", dataCols, always, identity, rows.Err()
}

func (dialect) SearchExpr(col, term string) string {
	return "CAST(" + col + " AS text) ILIKE " + sqlbase.QuoteLike(term, false)
}
