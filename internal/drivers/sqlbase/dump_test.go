package sqlbase

import (
	"bufio"
	"strings"
	"testing"
)

func split(t *testing.T, src string, o SplitOptions) []string {
	t.Helper()
	var out []string
	err := SplitStatements(bufio.NewReader(strings.NewReader(src)), o, func(s string) error {
		out = append(out, s)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSplitStatements(t *testing.T) {
	got := split(t, `
-- header comment; with semicolon
SELECT 'a;b', "c;d";
/* block; comment */ INSERT INTO t VALUES ('it''s');
SELECT 1--trailing
FROM x;
`, SplitOptions{})
	want := []string{`SELECT 'a;b', "c;d"`, `INSERT INTO t VALUES ('it''s')`, "SELECT 1\nFROM x"}
	if len(got) != len(want) {
		t.Fatalf("got %d statements: %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stmt %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitDollarQuotes(t *testing.T) {
	got := split(t, `DO $$ BEGIN CREATE TYPE x AS ENUM ('a'); EXCEPTION WHEN others THEN NULL; END $$;
CREATE FUNCTION f() RETURNS int AS $body$ SELECT 1; $body$ LANGUAGE sql;
SELECT $1;`, SplitOptions{DollarQuotes: true})
	if len(got) != 3 || !strings.HasSuffix(got[0], "END $$") || !strings.Contains(got[1], "SELECT 1; $body$") {
		t.Fatalf("got %q", got)
	}
}

func TestSplitBackslashes(t *testing.T) {
	got := split(t, `INSERT INTO t VALUES ('a\'b;c'); SELECT 2;`, SplitOptions{Backslashes: true})
	if len(got) != 2 || got[0] != `INSERT INTO t VALUES ('a\'b;c')` {
		t.Fatalf("got %q", got)
	}
}

func TestLiteral(t *testing.T) {
	my := LiteralStyle{TrueFalse: [2]string{"1", "0"}, Bytes: HexBytes, Backslashes: true}
	if got := my.Literal(`a\'b`); got != `'a\\''b'` {
		t.Errorf("mysql literal = %s", got)
	}
	if got := my.Literal([]byte{0, 255}); got != "X'00ff'" {
		t.Errorf("bytes literal = %s", got)
	}
	if got := my.Literal(nil); got != "NULL" {
		t.Errorf("nil literal = %s", got)
	}
}
