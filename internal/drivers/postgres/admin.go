package postgres

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/drivers/sqlbase"
)

// --- Explorer: server-level groups --------------------------------------

func serverGroups() []driver.Node {
	return []driver.Node{
		{Name: "Login/Group Roles", Kind: driver.KindGroup, Path: []string{driver.PathRoles}, HasKids: true},
		{Name: "Tablespaces", Kind: driver.KindGroup, Path: []string{driver.PathTablespaces}, HasKids: true},
	}
}

func isServerPath(path []string) bool {
	return len(path) > 0 && strings.HasPrefix(path[0], "\x01")
}

func (dialect) serverChildren(ctx context.Context, e *sqlbase.Engine, path []string) ([]driver.Node, error) {
	switch path[0] {
	case driver.PathRoles:
		pairs, err := sqlbase.Pairs(ctx, e.DB, `SELECT rolname, CASE WHEN rolcanlogin THEN 'login' ELSE 'group' END
			FROM pg_roles ORDER BY rolname LIKE 'pg\_%', rolname`)
		if err != nil {
			return nil, err
		}
		nodes := make([]driver.Node, len(pairs))
		for i, p := range pairs {
			nodes[i] = driver.Node{Name: p[0], Kind: driver.KindRole, Path: []string{driver.PathRoles, p[0]}, Detail: p[1]}
		}
		return nodes, nil
	case driver.PathTablespaces:
		pairs, err := sqlbase.Pairs(ctx, e.DB, `SELECT spcname, pg_get_userbyid(spcowner) FROM pg_tablespace ORDER BY spcname`)
		if err != nil {
			return nil, err
		}
		nodes := make([]driver.Node, len(pairs))
		for i, p := range pairs {
			nodes[i] = driver.Node{Name: p[0], Kind: driver.KindTablespace, Path: []string{driver.PathTablespaces, p[0]}, Detail: p[1]}
		}
		return nodes, nil
	}
	return nil, nil
}

// --- Roles ---------------------------------------------------------------

func (dialect) Roles(ctx context.Context, e *sqlbase.Engine) ([]driver.Role, error) {
	rows, err := e.DB.QueryContext(ctx, `SELECT r.rolname, r.rolcanlogin, r.rolsuper, r.rolcreatedb, r.rolcreaterole,
			r.rolinherit, r.rolreplication, r.rolbypassrls, r.rolconnlimit,
			COALESCE(to_char(r.rolvaliduntil AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
			COALESCE((SELECT string_agg(g.rolname, ',' ORDER BY g.rolname) FROM pg_auth_members m
			          JOIN pg_roles g ON g.oid = m.roleid WHERE m.member = r.oid), ''),
			COALESCE(shobj_description(r.oid, 'pg_authid'), '')
		FROM pg_roles r ORDER BY r.rolname LIKE 'pg\_%', r.rolname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []driver.Role
	for rows.Next() {
		var r driver.Role
		var members string
		if err := rows.Scan(&r.Name, &r.CanLogin, &r.Superuser, &r.CreateDB, &r.CreateRole, &r.Inherit,
			&r.Replication, &r.BypassRLS, &r.ConnLimit, &r.ValidUntil, &members, &r.Comment); err != nil {
			return nil, err
		}
		r.MemberOf = []string{}
		if members != "" {
			r.MemberOf = strings.Split(members, ",")
		}
		r.BuiltIn = strings.HasPrefix(r.Name, "pg_")
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d dialect) SaveRole(ctx context.Context, e *sqlbase.Engine, ch driver.RoleChange) error {
	var current []string
	if ch.OldName != "" {
		roles, err := d.Roles(ctx, e)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(roles, func(r driver.Role) bool { return r.Name == ch.OldName })
		if i < 0 {
			return fmt.Errorf("role %q no longer exists", ch.OldName)
		}
		current = roles[i].MemberOf
	}
	stmts, err := RoleStatements(ch, current)
	if err != nil {
		return err
	}
	tx, err := e.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	for _, s := range stmts {
		if _, err := tx.ExecContext(ctx, s); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (d dialect) DropRole(ctx context.Context, e *sqlbase.Engine, name string) error {
	if strings.HasPrefix(name, "pg_") {
		return errors.New("built-in roles can't be dropped")
	}
	_, err := e.DB.ExecContext(ctx, "DROP ROLE "+d.Quote(name))
	if err != nil && strings.Contains(err.Error(), "depend") {
		return fmt.Errorf("%w — reassign or drop the objects it owns first (REASSIGN OWNED BY / DROP OWNED BY)", err)
	}
	return err
}

// RoleStatements builds the SQL to create or update a role. currentMembers
// is the role's existing group membership (for updates).
func RoleStatements(ch driver.RoleChange, currentMembers []string) ([]string, error) {
	var d dialect
	r := ch.Role
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return nil, errors.New("give the role a name")
	}
	flag := func(on bool, yes, no string) string {
		if on {
			return yes
		}
		return no
	}
	opts := []string{
		flag(r.CanLogin, "LOGIN", "NOLOGIN"),
		flag(r.Superuser, "SUPERUSER", "NOSUPERUSER"),
		flag(r.CreateDB, "CREATEDB", "NOCREATEDB"),
		flag(r.CreateRole, "CREATEROLE", "NOCREATEROLE"),
		flag(r.Inherit, "INHERIT", "NOINHERIT"),
		flag(r.Replication, "REPLICATION", "NOREPLICATION"),
		flag(r.BypassRLS, "BYPASSRLS", "NOBYPASSRLS"),
		"CONNECTION LIMIT " + strconv.Itoa(max(r.ConnLimit, -1)),
	}
	if v := strings.TrimSpace(r.ValidUntil); v != "" {
		opts = append(opts, "VALID UNTIL "+literals.Str(v))
	} else {
		opts = append(opts, "VALID UNTIL 'infinity'")
	}
	if ch.SetPassword {
		if ch.Password == "" {
			opts = append(opts, "PASSWORD NULL")
		} else {
			opts = append(opts, "PASSWORD "+literals.Str(ch.Password))
		}
	}

	var stmts []string
	q := d.Quote(name)
	if ch.OldName == "" {
		stmts = append(stmts, "CREATE ROLE "+q+" WITH "+strings.Join(opts, " "))
	} else {
		if ch.OldName != name {
			stmts = append(stmts, "ALTER ROLE "+d.Quote(ch.OldName)+" RENAME TO "+q)
		}
		stmts = append(stmts, "ALTER ROLE "+q+" WITH "+strings.Join(opts, " "))
	}
	for _, g := range r.MemberOf {
		if !slices.Contains(currentMembers, g) {
			stmts = append(stmts, "GRANT "+d.Quote(g)+" TO "+q)
		}
	}
	for _, g := range currentMembers {
		if !slices.Contains(r.MemberOf, g) {
			stmts = append(stmts, "REVOKE "+d.Quote(g)+" FROM "+q)
		}
	}
	if c := strings.TrimSpace(r.Comment); c != "" {
		stmts = append(stmts, "COMMENT ON ROLE "+q+" IS "+literals.Str(c))
	} else if ch.OldName != "" {
		stmts = append(stmts, "COMMENT ON ROLE "+q+" IS NULL")
	}
	return stmts, nil
}

// --- Databases -----------------------------------------------------------

func (d dialect) CreateDatabase(ctx context.Context, e *sqlbase.Engine, name, owner string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("give the database a name")
	}
	stmt := "CREATE DATABASE " + d.Quote(name)
	if owner != "" {
		stmt += " OWNER " + d.Quote(owner)
	}
	_, err := e.DB.ExecContext(ctx, stmt)
	return err
}

func (d dialect) DropDatabase(ctx context.Context, e *sqlbase.Engine, name string) error {
	if strings.EqualFold(name, e.Default) {
		return errors.New("can't drop the database this connection is using — connect to another database (e.g. postgres) first")
	}
	_, err := e.DB.ExecContext(ctx, "DROP DATABASE "+d.Quote(name))
	if err != nil && strings.Contains(err.Error(), "other users") {
		return fmt.Errorf("%w — close other sessions using it first", err)
	}
	return err
}

// --- Structure -----------------------------------------------------------

func (d dialect) Structure(ctx context.Context, e *sqlbase.Engine, path []string) (*driver.TableStructure, error) {
	if len(path) != 3 || isServerPath(path) {
		return nil, errors.New("pick a table or view")
	}
	ref := d.Quote(path[1]) + "." + d.Quote(path[2])
	st := &driver.TableStructure{RowEstimate: -1}
	err := e.Scoped(ctx, path[0], func(q sqlbase.Querier) error {
		var relkind string
		var est float64
		if err := q.QueryRowContext(ctx, `SELECT c.relkind::text, c.reltuples,
				COALESCE(pg_size_pretty(pg_total_relation_size(c.oid)), ''), COALESCE(obj_description(c.oid, 'pg_class'), '')
			FROM pg_class c WHERE c.oid = $1::regclass`, ref).Scan(&relkind, &est, &st.Size, &st.Comment); err != nil {
			return err
		}
		if est >= 0 {
			st.RowEstimate = int64(est)
		}

		pk, _ := d.PrimaryKey(ctx, e, path)
		rows, err := q.QueryContext(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod), NOT a.attnotnull,
				COALESCE(pg_get_expr(ad.adbin, ad.adrelid), ''), COALESCE(col_description(a.attrelid, a.attnum), '')
			FROM pg_attribute a LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
			WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped ORDER BY a.attnum`, ref)
		if err != nil {
			return err
		}
		for rows.Next() {
			var c driver.StructColumn
			if err := rows.Scan(&c.Name, &c.Type, &c.Nullable, &c.Default, &c.Comment); err != nil {
				rows.Close()
				return err
			}
			c.PrimaryKey = slices.Contains(pk, c.Name)
			st.Columns = append(st.Columns, c)
		}
		rows.Close()

		if relkind == "v" || relkind == "m" {
			st.Kind = "view"
			var def string
			if err := q.QueryRowContext(ctx, `SELECT pg_get_viewdef($1::regclass, true)`, ref).Scan(&def); err != nil {
				return err
			}
			kw := "VIEW"
			if relkind == "m" {
				kw = "MATERIALIZED VIEW"
			}
			st.DDL = "CREATE " + kw + " " + ref + " AS\n" + strings.TrimSpace(def)
			return nil
		}

		st.Kind = "table"
		cons, err := q.QueryContext(ctx, `SELECT conname, contype::text, pg_get_constraintdef(oid)
			FROM pg_constraint WHERE conrelid = $1::regclass AND contype IN ('p','u','f','c','x')
			ORDER BY contype = 'p' DESC, conname`, ref)
		if err != nil {
			return err
		}
		kinds := map[string]string{"p": "primary key", "u": "unique", "f": "foreign key", "c": "check", "x": "exclusion"}
		for cons.Next() {
			var it driver.StructItem
			var k string
			if err := cons.Scan(&it.Name, &k, &it.Definition); err != nil {
				cons.Close()
				return err
			}
			it.Kind = kinds[k]
			st.Constraints = append(st.Constraints, it)
		}
		cons.Close()

		idx, err := q.QueryContext(ctx, `SELECT c.relname, CASE WHEN i.indisunique THEN 'unique index' ELSE 'index' END, pg_get_indexdef(i.indexrelid)
			FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE i.indrelid = $1::regclass ORDER BY c.relname`, ref)
		if err != nil {
			return err
		}
		for idx.Next() {
			var it driver.StructItem
			if err := idx.Scan(&it.Name, &it.Kind, &it.Definition); err != nil {
				idx.Close()
				return err
			}
			st.Indexes = append(st.Indexes, it)
		}
		idx.Close()

		ddl, _, _, _, err := d.createTable(ctx, q, ref)
		if err != nil {
			return err
		}
		var b strings.Builder
		b.WriteString(ddl + ";\n")
		for _, c := range st.Constraints {
			b.WriteString("\nALTER TABLE " + ref + " ADD CONSTRAINT " + d.Quote(c.Name) + " " + c.Definition + ";")
		}
		for _, ix := range st.Indexes {
			if !slices.ContainsFunc(st.Constraints, func(c driver.StructItem) bool { return c.Name == ix.Name }) {
				b.WriteString("\n" + ix.Definition + ";")
			}
		}
		if st.Comment != "" {
			b.WriteString("\n\nCOMMENT ON TABLE " + ref + " IS " + literals.Str(st.Comment) + ";")
		}
		st.DDL = strings.TrimSpace(b.String())
		return nil
	})
	return st, err
}
