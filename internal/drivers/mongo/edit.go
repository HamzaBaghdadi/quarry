package mongo

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// --- Editing -------------------------------------------------------------

func (c *conn) Keys(ctx context.Context, path []string) ([]string, error) {
	if len(path) != 2 {
		return nil, nil
	}
	specs, err := c.client.Database(path[0]).ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: path[1]}})
	if err != nil || len(specs) == 0 || specs[0].Type == "view" {
		return nil, err
	}
	return []string{"_id"}, nil
}

// toBSON converts a grid value back into a typed BSON value using the
// column's original type, so editing never silently turns an ObjectId or a
// date into a string.
func toBSON(v any, typ string) (any, error) {
	if v == nil {
		return nil, nil
	}
	s, isStr := v.(string)
	switch typ {
	case "objectId":
		if isStr {
			return bson.ObjectIDFromHex(strings.TrimSpace(s))
		}
	case "date":
		if isStr {
			t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
			if err != nil {
				return nil, fmt.Errorf("%q isn't an ISO date (e.g. 2025-01-31T09:00:00Z)", s)
			}
			return bson.NewDateTimeFromTime(t), nil
		}
	case "int":
		if f, ok := v.(float64); ok && f == math.Trunc(f) {
			if f >= math.MinInt32 && f <= math.MaxInt32 {
				return int32(f), nil
			}
			return int64(f), nil
		}
	case "string":
		return v, nil
	}
	if isStr && typ != "" && typ != "string" {
		// object, array, decimal, … or a value typed as JSON.
		return parseValue(s)
	}
	return v, nil
}

func parseValue(src string) (any, error) {
	js, err := toJSON(src)
	if err != nil {
		return nil, err
	}
	var w struct {
		V any `bson:"v"`
	}
	if err := bson.UnmarshalExtJSON([]byte(`{"v":`+js+`}`), false, &w); err != nil {
		return nil, fmt.Errorf("invalid value %q: %w", src, err)
	}
	return w.V, nil
}

func convertDoc(m map[string]any, types map[string]string) (bson.D, error) {
	d := bson.D{}
	for k, v := range m {
		bv, err := toBSON(v, types[k])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		d = append(d, bson.E{Key: k, Value: bv})
	}
	return d, nil
}

func (c *conn) Apply(ctx context.Context, cs driver.ChangeSet) (*driver.Result, error) {
	t := driver.StartTimer()
	if len(cs.Path) != 2 {
		return nil, errors.New("pick a collection")
	}
	coll := c.client.Database(cs.Path[0]).Collection(cs.Path[1])
	// Mongo transactions need a replica set, so writes are applied one by
	// one; each must match exactly one document.
	var done int64
	fail := func(err error) (*driver.Result, error) {
		if done > 0 {
			return nil, fmt.Errorf("%w (%d earlier change(s) were already saved)", err, done)
		}
		return nil, err
	}
	for _, del := range cs.Deletes {
		f, err := convertDoc(del, cs.Types)
		if err != nil {
			return fail(err)
		}
		r, err := coll.DeleteOne(ctx, f)
		if err != nil {
			return fail(err)
		}
		if r.DeletedCount != 1 {
			return fail(fmt.Errorf("document %v not found", del["_id"]))
		}
		done++
	}
	for _, u := range cs.Updates {
		f, err := convertDoc(u.Key, cs.Types)
		if err != nil {
			return fail(err)
		}
		set, err := convertDoc(u.Values, cs.Types)
		if err != nil {
			return fail(err)
		}
		r, err := coll.UpdateOne(ctx, f, bson.D{{Key: "$set", Value: set}})
		if err != nil {
			return fail(err)
		}
		if r.MatchedCount != 1 {
			return fail(fmt.Errorf("document %v not found", u.Key["_id"]))
		}
		done++
	}
	for _, row := range cs.Inserts {
		doc, err := convertDoc(row, cs.Types)
		if err != nil {
			return fail(err)
		}
		if _, err := coll.InsertOne(ctx, doc); err != nil {
			return fail(err)
		}
		done++
	}
	return t.Stamp(&driver.Result{RowsAffected: done, Message: fmt.Sprintf("Saved %d change(s)", done)}), nil
}

// --- Backup --------------------------------------------------------------
//
// Format: newline-delimited JSON.
//   {"quarry":"mongodb-backup","version":1,"database":"app"}
//   {"collection":"users","indexes":[…]}
//   {"c":"users","d":{…canonical Extended JSON…}}

func (c *conn) BackupFormat() string { return "ndjson" }

type header struct {
	Quarry   string `json:"quarry"`
	Version  int    `json:"version"`
	Database string `json:"database,omitempty"`
}

type collLine struct {
	Collection string            `json:"collection"`
	Indexes    []json.RawMessage `json:"indexes,omitempty"`
}

type docLine struct {
	C string          `json:"c"`
	D json.RawMessage `json:"d"`
}

func (c *conn) dbName(scope string) (string, error) {
	if scope == "" {
		scope = c.defaultDB
	}
	if scope == "" {
		return "", errors.New("pick a database")
	}
	return scope, nil
}

func (c *conn) Backup(ctx context.Context, scope string, w io.Writer, p driver.Progress) (driver.BackupStats, error) {
	var st driver.BackupStats
	name, err := c.dbName(scope)
	if err != nil {
		return st, err
	}
	db := c.client.Database(name)
	bw := bufio.NewWriterSize(w, 1<<16)
	enc := json.NewEncoder(bw)
	_ = enc.Encode(header{Quarry: "mongodb-backup", Version: 1, Database: name})

	specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "type", Value: "collection"}})
	if err != nil {
		return st, err
	}
	for i, s := range specs {
		if strings.HasPrefix(s.Name, "system.") {
			continue
		}
		p(fmt.Sprintf("Collection %d/%d · %s", i+1, len(specs), s.Name))
		coll := db.Collection(s.Name)
		line := collLine{Collection: s.Name}
		if cur, err := coll.Indexes().List(ctx); err == nil {
			var idx []bson.D
			if err := cur.All(ctx, &idx); err == nil {
				for _, ix := range idx {
					if b, err := bson.MarshalExtJSON(ix, true, false); err == nil {
						line.Indexes = append(line.Indexes, b)
					}
				}
			}
		}
		_ = enc.Encode(line)
		cur, err := coll.Find(ctx, bson.D{})
		if err != nil {
			return st, err
		}
		for cur.Next(ctx) {
			b, err := bson.MarshalExtJSON(cur.Current, true, false)
			if err != nil {
				cur.Close(ctx)
				return st, err
			}
			if err := enc.Encode(docLine{C: s.Name, D: b}); err != nil {
				cur.Close(ctx)
				return st, err
			}
			st.Rows++
			if st.Rows%5000 == 0 {
				p(fmt.Sprintf("%s · %d documents", s.Name, st.Rows))
			}
		}
		if err := cur.Err(); err != nil {
			cur.Close(ctx)
			return st, err
		}
		cur.Close(ctx)
		st.Objects++
	}
	return st, bw.Flush()
}

func (c *conn) Restore(ctx context.Context, scope string, r io.Reader, p driver.Progress) (driver.BackupStats, error) {
	var st driver.BackupStats
	name, err := c.dbName(scope)
	if err != nil {
		return st, err
	}
	db := c.client.Database(name)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20) // documents can be up to 16 MB

	if !sc.Scan() {
		return st, errors.New("empty backup file")
	}
	var h header
	if json.Unmarshal(sc.Bytes(), &h) != nil || h.Quarry != "mongodb-backup" {
		return st, errors.New("this isn't a Quarry MongoDB backup")
	}

	var batch []any
	current := ""
	indexes := map[string][]json.RawMessage{}
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := db.Collection(current).InsertMany(ctx, batch, options.InsertMany().SetOrdered(false))
		st.Rows += int64(len(batch))
		st.Statements++
		batch = batch[:0]
		p(fmt.Sprintf("%s · %d documents", current, st.Rows))
		return err
	}

	for sc.Scan() {
		raw := sc.Bytes()
		if len(raw) == 0 {
			continue
		}
		if strings.HasPrefix(string(raw), `{"collection"`) {
			if err := flush(); err != nil {
				return st, err
			}
			var cl collLine
			if err := json.Unmarshal(raw, &cl); err != nil {
				return st, err
			}
			current = cl.Collection
			indexes[current] = cl.Indexes
			if err := db.Collection(current).Drop(ctx); err != nil {
				return st, fmt.Errorf("drop %s: %w", current, err)
			}
			st.Objects++
			continue
		}
		var dl docLine
		if err := json.Unmarshal(raw, &dl); err != nil {
			return st, err
		}
		if dl.C != current {
			if err := flush(); err != nil {
				return st, err
			}
			current = dl.C
		}
		var doc bson.D
		if err := bson.UnmarshalExtJSON(dl.D, true, &doc); err != nil {
			return st, err
		}
		batch = append(batch, doc)
		if len(batch) >= 500 {
			if err := flush(); err != nil {
				return st, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return st, err
	}
	if err := flush(); err != nil {
		return st, err
	}

	for coll, specs := range indexes {
		var list bson.A
		for _, raw := range specs {
			var ix bson.D
			if bson.UnmarshalExtJSON(raw, true, &ix) != nil {
				continue
			}
			clean := bson.D{}
			skip := false
			for _, e := range ix {
				switch e.Key {
				case "v", "ns":
					continue
				case "name":
					skip = e.Value == "_id_"
				}
				clean = append(clean, e)
			}
			if !skip {
				list = append(list, clean)
			}
		}
		if len(list) == 0 {
			continue
		}
		p("Rebuilding indexes on " + coll)
		cmd := bson.D{{Key: "createIndexes", Value: coll}, {Key: "indexes", Value: list}}
		if err := db.RunCommand(ctx, cmd).Err(); err != nil {
			return st, fmt.Errorf("indexes on %s: %w", coll, err)
		}
	}
	return st, nil
}

