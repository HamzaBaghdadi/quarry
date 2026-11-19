package mongo

import "testing"

func TestToJSON(t *testing.T) {
	cases := map[string]string{
		`{ age: { $gt: 30 }, name: 'Ada' }`:     `{ "age": { "$gt": 30 }, "name": "Ada" }`,
		`{ _id: ObjectId("64b7f0c2a1b2c3d4e5f60718") }`: `{ "_id": {"$oid":"64b7f0c2a1b2c3d4e5f60718"} }`,
		`[1, 2, 3,]`:                              `[1, 2, 3]`,
		`{ n: NumberLong(5) }`:                    `{ "n": {"$numberLong":"5"} }`,
		`{ "already": "json" }`:                   `{ "already": "json" }`,
	}
	for in, want := range cases {
		got, err := toJSON(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Errorf("toJSON(%s)\n got %s\nwant %s", in, got, want)
		}
	}
}

func TestParseShell(t *testing.T) {
	e, err := parseShell(`db.users.find({ age: { $gt: 30 } }).sort({ name: 1 }).limit(10);`)
	if err != nil {
		t.Fatal(err)
	}
	if e.Collection != "users" || len(e.Calls) != 3 || e.Calls[0].Name != "find" || e.Calls[2].Args != "10" {
		t.Fatalf("unexpected parse: %+v", e)
	}
	e, err = parseShell(`db.getCollection("weird.name").aggregate([{ $match: {} }])`)
	if err != nil {
		t.Fatal(err)
	}
	if e.Collection != "weird.name" || e.Calls[0].Name != "aggregate" {
		t.Fatalf("unexpected parse: %+v", e)
	}
	if _, err := parseShell(`users.find()`); err == nil {
		t.Fatal("expected error for missing db. prefix")
	}
}

func TestArgs(t *testing.T) {
	a, err := args(`{ status: 'active' }, { name: 1 }`)
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 2 {
		t.Fatalf("want 2 args, got %d", len(a))
	}
}
