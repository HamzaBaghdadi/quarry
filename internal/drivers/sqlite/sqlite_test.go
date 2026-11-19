package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// End-to-end test of the shared SQL engine through the SQLite adapter.
func TestSQLiteRoundTrip(t *testing.T) {
	ctx := context.Background()
	conn, err := Driver{}.Open(ctx, driver.Config{File: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Execute(ctx, "", "CREATE TABLE people (id INTEGER PRIMARY KEY, name TEXT, score REAL)"); err != nil {
		t.Fatal(err)
	}
	res, err := conn.Execute(ctx, "", "INSERT INTO people (name, score) VALUES ('Ada', 9.5), ('Linus', 8), ('Grace', NULL)")
	if err != nil {
		t.Fatal(err)
	}
	if res.RowsAffected != 3 {
		t.Fatalf("rows affected = %d", res.RowsAffected)
	}

	nodes, err := conn.Children(ctx, nil)
	if err != nil || len(nodes) != 1 || nodes[0].Name != "people" || !nodes[0].Browsable {
		t.Fatalf("children = %+v, %v", nodes, err)
	}

	page, err := conn.Browse(ctx, driver.BrowseRequest{Path: nodes[0].Path, Limit: 2, OrderBy: "name"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 2 || !page.HasMore || page.Rows[0][1] != "Ada" {
		t.Fatalf("page = %+v", page)
	}

	filtered, err := conn.Browse(ctx, driver.BrowseRequest{Path: nodes[0].Path, Limit: 10, Filter: "score IS NULL"})
	if err != nil || len(filtered.Rows) != 1 || filtered.Rows[0][1] != "Grace" {
		t.Fatalf("filtered = %+v, %v", filtered, err)
	}

	sel, err := conn.Execute(ctx, "", "-- comment first\nSELECT count(*) AS n FROM people")
	if err != nil || len(sel.Columns) != 1 || sel.Rows[0][0] != int64(3) {
		t.Fatalf("select = %+v, %v", sel, err)
	}
}
