// Package driver defines the engine-agnostic contract every database adapter
// implements. The UI only ever talks to these interfaces, so adding support for
// a new database means writing one adapter and registering it — nothing else in
// the app changes (Strategy + Registry patterns).
package driver

import (
	"context"
	"time"
)

// Family groups engines that share a data model. The UI uses it to pick
// sensible defaults (editor language, result rendering).
type Family string

const (
	FamilySQL      Family = "sql"
	FamilyDocument Family = "document"
	FamilyKeyValue Family = "keyvalue"
)

// Field names a connection form field. Each driver advertises which fields it
// needs so the UI can render the right form without hard-coding engines.
type Field string

const (
	FieldHost     Field = "host"
	FieldPort     Field = "port"
	FieldUser     Field = "user"
	FieldPassword Field = "password"
	FieldDatabase Field = "database"
	FieldFile     Field = "file"
	FieldURI      Field = "uri"
	FieldSSL      Field = "ssl"
)

// Info describes a driver to the UI.
type Info struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	Family         Family  `json:"family"`
	Language       string  `json:"language"` // editor language: sql | mongodb | redis
	DefaultPort    int     `json:"defaultPort"`
	DefaultUser    string  `json:"defaultUser"`
	Fields         []Field `json:"fields"`
	URIPlaceholder string  `json:"uriPlaceholder"`
	SSLModes       []string `json:"sslModes"`
	Example        string  `json:"example"` // starter query for a new editor tab
	// Capabilities lists optional features: CapEdit, CapBackup.
	Capabilities []string `json:"capabilities"`
}

// Config is everything needed to open a connection.
type Config struct {
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	User     string            `json:"user"`
	Password string            `json:"password"`
	Database string            `json:"database"`
	File     string            `json:"file"`
	URI      string            `json:"uri"`
	SSLMode  string            `json:"sslMode"`
	Options  map[string]string `json:"options"`
}

// Driver is a factory for live connections to one kind of database.
type Driver interface {
	Info() Info
	Open(ctx context.Context, cfg Config) (Conn, error)
}

// Conn is a live, concurrency-safe connection to a database server.
type Conn interface {
	Ping(ctx context.Context) error
	Close() error
	// Children lists explorer nodes beneath path (nil path = root).
	Children(ctx context.Context, path []string) ([]Node, error)
	// Browse reads a page of rows from a browsable node (table, collection, key).
	Browse(ctx context.Context, req BrowseRequest) (*Result, error)
	// Execute runs a query in the engine's native language. scope is the
	// database (or db index) the editor tab is pointed at; "" means default.
	Execute(ctx context.Context, scope string, query string) (*Result, error)
}

// NodeKind tells the UI which icon to show and how to treat a node.
type NodeKind string

const (
	KindDatabase   NodeKind = "database"
	KindSchema     NodeKind = "schema"
	KindTable      NodeKind = "table"
	KindView       NodeKind = "view"
	KindCollection NodeKind = "collection"
	KindKey        NodeKind = "key"
	KindGroup      NodeKind = "group"
	KindRole       NodeKind = "role"
	KindTablespace NodeKind = "tablespace"
)

// Node is one entry in the explorer tree. Path identifies it uniquely within
// a connection and is passed back verbatim to Children / Browse.
type Node struct {
	Name      string   `json:"name"`
	Kind      NodeKind `json:"kind"`
	Path      []string `json:"path"`
	HasKids   bool     `json:"hasChildren"`
	Browsable bool     `json:"browsable"`
	Detail    string   `json:"detail,omitempty"` // e.g. key type, row estimate
}

// BrowseRequest asks for a page of data from a node.
type BrowseRequest struct {
	Path    []string `json:"path"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
	Filter  string   `json:"filter"`  // SQL WHERE clause / Mongo JSON filter / Redis pattern
	Search  string   `json:"search"`  // free text matched against every column
	OrderBy string   `json:"orderBy"` // column / field name
	Desc    bool     `json:"desc"`
}

// Column describes one result column.
type Column struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Result is the uniform shape of every query/browse response.
type Result struct {
	Columns      []Column `json:"columns"`
	Rows         [][]any  `json:"rows"`
	RowsAffected int64    `json:"rowsAffected"`
	HasMore      bool     `json:"hasMore"`
	Message      string   `json:"message,omitempty"`
	DurationMs   float64  `json:"durationMs"`
	Statement    string   `json:"statement,omitempty"`
	// Documents holds pretty JSON per row for document stores, so the UI can
	// offer a lossless JSON view next to the flattened grid.
	Documents []string `json:"documents,omitempty"`
	// Keys are the columns that identify a row when the result is editable
	// (set on Browse results only). Empty means read-only.
	Keys []string `json:"keys,omitempty"`
	// EditBlocked explains why a browsed object can't be edited.
	EditBlocked string `json:"editBlocked,omitempty"`
}

// Timer is a tiny helper for stamping Result.DurationMs.
type Timer struct{ start time.Time }

func StartTimer() Timer { return Timer{start: time.Now()} }

func (t Timer) Stamp(r *Result) *Result {
	if r != nil {
		r.DurationMs = float64(time.Since(t.start).Microseconds()) / 1000
	}
	return r
}
