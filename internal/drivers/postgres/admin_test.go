package postgres

import (
	"strings"
	"testing"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

func TestRoleStatementsCreate(t *testing.T) {
	stmts, err := RoleStatements(driver.RoleChange{
		Role:        driver.Role{Name: "app_user", CanLogin: true, Inherit: true, ConnLimit: 10, MemberOf: []string{"readers"}, Comment: "it's the app"},
		SetPassword: true, Password: "s3cr'et",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(stmts, ";\n")
	for _, want := range []string{
		`CREATE ROLE "app_user" WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE INHERIT NOREPLICATION NOBYPASSRLS CONNECTION LIMIT 10 VALID UNTIL 'infinity' PASSWORD 's3cr''et'`,
		`GRANT "readers" TO "app_user"`,
		`COMMENT ON ROLE "app_user" IS 'it''s the app'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRoleStatementsUpdate(t *testing.T) {
	stmts, err := RoleStatements(driver.RoleChange{
		OldName: "old",
		Role:    driver.Role{Name: "new", ConnLimit: -1, ValidUntil: "2030-01-01T00:00:00Z", MemberOf: []string{"b", "c"}},
	}, []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(stmts, ";\n")
	for _, want := range []string{
		`ALTER ROLE "old" RENAME TO "new"`,
		`ALTER ROLE "new" WITH NOLOGIN`,
		`VALID UNTIL '2030-01-01T00:00:00Z'`,
		`GRANT "c" TO "new"`,
		`REVOKE "a" FROM "new"`,
		`COMMENT ON ROLE "new" IS NULL`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "PASSWORD") {
		t.Error("password must not change unless SetPassword is true")
	}
	if strings.Contains(got, `GRANT "b"`) {
		t.Error("existing membership must not be re-granted")
	}
}

func TestRoleStatementsRequiresName(t *testing.T) {
	if _, err := RoleStatements(driver.RoleChange{Role: driver.Role{Name: "  "}}, nil); err == nil {
		t.Fatal("expected error for empty name")
	}
}
