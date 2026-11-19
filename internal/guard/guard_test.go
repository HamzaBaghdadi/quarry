package guard

import "testing"

func TestReadOnly(t *testing.T) {
	allowed := []struct{ lang, q string }{
		{"sql", "SELECT * FROM users WHERE note = 'drop table'"},
		{"sql", "WITH x AS (SELECT 1) SELECT * FROM x"},
		{"mongodb", `db.users.find({ status: "active" })`},
		{"redis", "GET user:1\nHGETALL session:2"},
	}
	for _, c := range allowed {
		if err := ReadOnly(c.lang, c.q); err != nil {
			t.Errorf("%s %q should be allowed: %v", c.lang, c.q, err)
		}
	}
	blocked := []struct{ lang, q string }{
		{"sql", "DELETE FROM users"},
		{"sql", "select 1; drop table users"},
		{"mongodb", `db.users.deleteMany({})`},
		{"mongodb", `db.users.aggregate([{ $out: "x" }])`},
		{"redis", "SET k v"},
		{"redis", "FLUSHALL"},
	}
	for _, c := range blocked {
		if err := ReadOnly(c.lang, c.q); err == nil {
			t.Errorf("%s %q should be blocked", c.lang, c.q)
		}
	}
}
