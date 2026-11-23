package driver

import "context"

// Server administration capabilities (pgAdmin-style). Like the other optional
// features, a Conn opts in by implementing the interface and the driver
// advertises it in Info.Capabilities.

const (
	CapRoles     = "roles"
	CapDatabases = "databases"
	CapStructure = "structure"
)

// Explorer paths for server-level groups. They start with \x01 so they can't
// collide with a real database name.
const (
	PathRoles       = "\x01roles"
	PathTablespaces = "\x01tablespaces"
)

// Role is a login or group role.
type Role struct {
	Name        string   `json:"name"`
	CanLogin    bool     `json:"canLogin"`
	Superuser   bool     `json:"superuser"`
	CreateDB    bool     `json:"createDb"`
	CreateRole  bool     `json:"createRole"`
	Inherit     bool     `json:"inherit"`
	Replication bool     `json:"replication"`
	BypassRLS   bool     `json:"bypassRls"`
	ConnLimit   int      `json:"connLimit"`  // -1 = unlimited
	ValidUntil  string   `json:"validUntil"` // "" = never
	MemberOf    []string `json:"memberOf"`
	Comment     string   `json:"comment"`
	BuiltIn     bool     `json:"builtIn"` // pg_* system roles
}

// RoleChange describes a create or update. OldName is empty for a new role;
// Password is applied only when SetPassword is true ("" with SetPassword
// removes the password).
type RoleChange struct {
	OldName     string `json:"oldName"`
	Role        Role   `json:"role"`
	SetPassword bool   `json:"setPassword"`
	Password    string `json:"password"`
}

// RoleManager manages server roles.
type RoleManager interface {
	Roles(ctx context.Context) ([]Role, error)
	SaveRole(ctx context.Context, ch RoleChange) error
	DropRole(ctx context.Context, name string) error
}

// DatabaseManager creates and drops databases.
type DatabaseManager interface {
	CreateDatabase(ctx context.Context, name, owner string) error
	DropDatabase(ctx context.Context, name string) error
}

// StructColumn is one column in a table's structure.
type StructColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Nullable   bool   `json:"nullable"`
	Default    string `json:"default"`
	PrimaryKey bool   `json:"primaryKey"`
	Comment    string `json:"comment"`
}

// StructItem is an index or constraint.
type StructItem struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"` // primary key, unique, foreign key, check, index…
	Definition string `json:"definition"`
}

// TableStructure is the Properties/SQL view of a table or view.
type TableStructure struct {
	Kind        string         `json:"kind"` // table | view
	Columns     []StructColumn `json:"columns"`
	Indexes     []StructItem   `json:"indexes"`
	Constraints []StructItem   `json:"constraints"`
	DDL         string         `json:"ddl"`
	RowEstimate int64          `json:"rowEstimate"` // -1 if unknown
	Size        string         `json:"size"`
	Comment     string         `json:"comment"`
}

// Structurer describes tables.
type Structurer interface {
	Structure(ctx context.Context, path []string) (*TableStructure, error)
}
