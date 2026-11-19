package sqlite

import (
	"bytes"
	"fmt"
	"context"
	"path/filepath"
	"testing"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

func open(t *testing.T) driver.Conn {
	t.Helper()
	c, err := Driver{}.Open(context.Background(), driver.Config{File: filepath.Join(t.TempDir(), "x.db")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func mustExec(t *testing.T, c driver.Conn, q string) *driver.Result {
	t.Helper()
	r, err := c.Execute(context.Background(), "", q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	return r
}

func TestEditRoundTrip(t *testing.T) {
	ctx := context.Background()
	c := open(t)
	mustExec(t, c, "CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT, n INTEGER DEFAULT 7)")
	mustExec(t, c, "INSERT INTO t (id, name) VALUES (1, 'a'), (2, 'b'), (3, 'c')")

	ed := c.(driver.Editor)
	keys, err := ed.Keys(ctx, []string{"t"})
	if err != nil || len(keys) != 1 || keys[0] != "id" {
		t.Fatalf("keys = %v, %v", keys, err)
	}
	_, err = ed.Apply(ctx, driver.ChangeSet{
		Path:    []string{"t"},
		Updates: []driver.RowChange{{Key: map[string]any{"id": float64(1)}, Values: map[string]any{"name": "A", "n": nil}}},
		Inserts: []map[string]any{{"id": float64(4), "name": "d"}, {}},
		Deletes: []map[string]any{{"id": float64(2)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	r := mustExec(t, c, "SELECT id, name, n FROM t ORDER BY id")
	got := ""
	for _, row := range r.Rows {
		got += fmt.Sprintf("%v,%v,%v;", row[0], row[1], row[2])
	}
	if got != "1,A,<nil>;3,c,7;4,d,7;5,<nil>,7;" {
		t.Fatalf("rows after edit = %s", got)
	}

	// An update whose key no longer matches must roll back everything.
	_, err = ed.Apply(ctx, driver.ChangeSet{
		Path:    []string{"t"},
		Updates: []driver.RowChange{{Key: map[string]any{"id": float64(3)}, Values: map[string]any{"name": "changed"}}, {Key: map[string]any{"id": float64(99)}, Values: map[string]any{"name": "x"}}},
	})
	if err == nil {
		t.Fatal("expected error for missing row")
	}
	if v := mustExec(t, c, "SELECT name FROM t WHERE id = 3").Rows[0][0]; v != "c" {
		t.Fatalf("partial change leaked: %v", v)
	}
}

func TestBackupRestore(t *testing.T) {
	ctx := context.Background()
	src := open(t)
	mustExec(t, src, "CREATE TABLE parent (id INTEGER PRIMARY KEY, label TEXT NOT NULL, blob BLOB, score REAL)")
	mustExec(t, src, "CREATE TABLE child (id INTEGER PRIMARY KEY, parent_id INTEGER REFERENCES parent(id), note TEXT)")
	mustExec(t, src, "CREATE INDEX child_parent ON child(parent_id)")
	mustExec(t, src, "CREATE VIEW v AS SELECT p.label, c.note FROM parent p JOIN child c ON c.parent_id = p.id")
	mustExec(t, src, `INSERT INTO parent VALUES (1, 'it''s; tricky -- not a comment', X'00ff10', 1.5), (2, 'back\slash', NULL, NULL)`)
	for i := 0; i < 450; i++ { // crosses several insert batches
		mustExec(t, src, "INSERT INTO child (parent_id, note) VALUES (1, 'n')")
	}

	var buf bytes.Buffer
	b := src.(driver.Backuper)
	st, err := b.Backup(ctx, "", &buf, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if st.Objects != 2 || st.Rows != 452 {
		t.Fatalf("backup stats = %+v", st)
	}

	dst := open(t)
	mustExec(t, dst, "CREATE TABLE parent (junk TEXT)") // replaced by restore
	if _, err := dst.(driver.Backuper).Restore(ctx, "", bytes.NewReader(buf.Bytes()), func(string) {}); err != nil {
		t.Fatalf("restore: %v\n%s", err, buf.String()[:min(buf.Len(), 2000)])
	}
	r := mustExec(t, dst, "SELECT label, hex(blob), score FROM parent ORDER BY id")
	if r.Rows[0][0] != "it's; tricky -- not a comment" || r.Rows[0][1] != "00FF10" || r.Rows[1][0] != `back\slash` {
		t.Fatalf("parent rows = %v", r.Rows)
	}
	if n := mustExec(t, dst, "SELECT count(*) FROM v").Rows[0][0]; n != int64(450) {
		t.Fatalf("view rows = %v", n)
	}
	if n := mustExec(t, dst, "SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = 'child_parent'").Rows[0][0]; n != int64(1) {
		t.Fatal("index not restored")
	}
}
