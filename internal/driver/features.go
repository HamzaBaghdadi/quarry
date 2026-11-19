package driver

import (
	"context"
	"io"
)

// Optional capabilities. A Conn opts in by implementing these interfaces; the
// app discovers them with a type assertion, so engines that can't support a
// feature simply don't implement it.

const (
	CapEdit   = "edit"
	CapBackup = "backup"
)

// RowChange updates one row, identified by Key (column → current value).
// A nil value in Values means NULL.
type RowChange struct {
	Key    map[string]any `json:"key"`
	Values map[string]any `json:"values"`
}

// ChangeSet is a batch of edits to one table/collection/key, applied atomically
// where the engine allows it.
type ChangeSet struct {
	Path    []string          `json:"path"`
	Updates []RowChange       `json:"updates"`
	Inserts []map[string]any  `json:"inserts"`
	Deletes []map[string]any  `json:"deletes"` // row keys
	Types   map[string]string `json:"types"`   // column → type, as reported in Result.Columns
}

// Editor is implemented by connections that can change data in place.
type Editor interface {
	// Keys returns the columns that uniquely identify a row of path.
	// An empty slice means the object can't be edited safely.
	Keys(ctx context.Context, path []string) ([]string, error)
	// Apply writes a change set. Every update and delete must touch exactly
	// one row, otherwise nothing is saved.
	Apply(ctx context.Context, cs ChangeSet) (*Result, error)
}

// Progress reports backup/restore progress in human terms.
type Progress func(message string)

// BackupStats summarises a backup or restore.
type BackupStats struct {
	Objects    int   `json:"objects"`    // tables, collections or keys
	Rows       int64 `json:"rows"`       // rows / documents / keys written
	Statements int   `json:"statements"` // statements executed on restore
}

// Backuper is implemented by connections that can dump and reload a database
// (scope) without external tools.
type Backuper interface {
	// BackupFormat is the file extension of the dump ("sql", "ndjson").
	BackupFormat() string
	Backup(ctx context.Context, scope string, w io.Writer, p Progress) (BackupStats, error)
	Restore(ctx context.Context, scope string, r io.Reader, p Progress) (BackupStats, error)
}
