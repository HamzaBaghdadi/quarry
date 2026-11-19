package redis

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

// --- Editing -------------------------------------------------------------
//
// Each key type is shown as a small table (see Browse); its "key column" is
// what identifies an entry inside the Redis key.

var keyColumn = map[string]string{"string": "value", "hash": "field", "list": "index", "set": "member", "zset": "member"}

func (c *conn) Keys(ctx context.Context, path []string) ([]string, error) {
	if len(path) != 2 {
		return nil, nil
	}
	typ, err := c.client(dbIndex(path[0])).Type(ctx, path[1]).Result()
	if err != nil {
		return nil, err
	}
	if col, ok := keyColumn[typ]; ok {
		return []string{col}, nil
	}
	return nil, nil
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

func num(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case string:
		return strconv.ParseFloat(x, 64)
	}
	return 0, fmt.Errorf("%v isn't a number", v)
}

func (c *conn) Apply(ctx context.Context, cs driver.ChangeSet) (*driver.Result, error) {
	t := driver.StartTimer()
	if len(cs.Path) != 2 {
		return nil, errors.New("pick a key")
	}
	cl := c.client(dbIndex(cs.Path[0]))
	key := cs.Path[1]
	typ, err := cl.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	// Reads needed before writing (old values for renames).
	var pre []func(goredis.Pipeliner) error
	n := 0
	for _, u := range cs.Updates {
		u := u
		n++
		switch typ {
		case "string":
			pre = append(pre, func(p goredis.Pipeliner) error {
				return p.SetArgs(ctx, key, str(u.Values["value"]), goredis.SetArgs{KeepTTL: true}).Err()
			})
		case "hash":
			field := str(u.Key["field"])
			newField, renamed := u.Values["field"]
			val, hasVal := u.Values["value"]
			if !hasVal {
				v, err := cl.HGet(ctx, key, field).Result()
				if err != nil {
					return nil, fmt.Errorf("field %q: %w", field, err)
				}
				val = v
			}
			pre = append(pre, func(p goredis.Pipeliner) error {
				if renamed && str(newField) != field {
					p.HDel(ctx, key, field)
					field = str(newField)
				}
				return p.HSet(ctx, key, field, str(val)).Err()
			})
		case "list":
			idx, err := num(u.Key["index"])
			if err != nil {
				return nil, err
			}
			if _, ok := u.Values["index"]; ok {
				return nil, errors.New("list indexes can't be edited — change the value instead")
			}
			pre = append(pre, func(p goredis.Pipeliner) error {
				return p.LSet(ctx, key, int64(idx), str(u.Values["value"])).Err()
			})
		case "set":
			old, nw := str(u.Key["member"]), str(u.Values["member"])
			pre = append(pre, func(p goredis.Pipeliner) error {
				p.SRem(ctx, key, old)
				return p.SAdd(ctx, key, nw).Err()
			})
		case "zset":
			old := str(u.Key["member"])
			member := old
			if m, ok := u.Values["member"]; ok {
				member = str(m)
			}
			var score float64
			if s, ok := u.Values["score"]; ok {
				if score, err = num(s); err != nil {
					return nil, err
				}
			} else if score, err = cl.ZScore(ctx, key, old).Result(); err != nil {
				return nil, err
			}
			pre = append(pre, func(p goredis.Pipeliner) error {
				if member != old {
					p.ZRem(ctx, key, old)
				}
				return p.ZAdd(ctx, key, goredis.Z{Score: score, Member: member}).Err()
			})
		default:
			return nil, fmt.Errorf("%s keys can't be edited", typ)
		}
	}
	const tomb = "\x00quarry:deleted\x00"
	listDeletes := 0
	for _, d := range cs.Deletes {
		d := d
		n++
		switch typ {
		case "string":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.Del(ctx, key).Err() })
		case "hash":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.HDel(ctx, key, str(d["field"])).Err() })
		case "list":
			idx, err := num(d["index"])
			if err != nil {
				return nil, err
			}
			listDeletes++
			pre = append(pre, func(p goredis.Pipeliner) error { return p.LSet(ctx, key, int64(idx), tomb).Err() })
		case "set":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.SRem(ctx, key, str(d["member"])).Err() })
		case "zset":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.ZRem(ctx, key, str(d["member"])).Err() })
		}
	}
	if listDeletes > 0 {
		pre = append(pre, func(p goredis.Pipeliner) error { return p.LRem(ctx, key, 0, tomb).Err() })
	}
	for _, row := range cs.Inserts {
		row := row
		n++
		switch typ {
		case "hash":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.HSet(ctx, key, str(row["field"]), str(row["value"])).Err() })
		case "list":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.RPush(ctx, key, str(row["value"])).Err() })
		case "set":
			pre = append(pre, func(p goredis.Pipeliner) error { return p.SAdd(ctx, key, str(row["member"])).Err() })
		case "zset":
			score := 0.0
			if s, ok := row["score"]; ok && s != nil {
				if score, err = num(s); err != nil {
					return nil, err
				}
			}
			pre = append(pre, func(p goredis.Pipeliner) error {
				return p.ZAdd(ctx, key, goredis.Z{Score: score, Member: str(row["member"])}).Err()
			})
		default:
			return nil, fmt.Errorf("can't add rows to a %s key", typ)
		}
	}

	// Apply everything atomically.
	_, err = cl.TxPipelined(ctx, func(p goredis.Pipeliner) error {
		for _, f := range pre {
			if err := f(p); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t.Stamp(&driver.Result{RowsAffected: int64(n), Message: fmt.Sprintf("Saved %d change(s)", n)}), nil
}

// --- Backup --------------------------------------------------------------
//
// Format: newline-delimited JSON of DUMP payloads, restorable with RESTORE on
// the same or a newer server version.
//   {"quarry":"redis-backup","version":1,"db":0}
//   {"k":"<base64 key>","v":"<base64 DUMP>","t":<pttl ms, 0 = none>}

func (c *conn) BackupFormat() string { return "ndjson" }

type entry struct {
	K string `json:"k"`
	V string `json:"v"`
	T int64  `json:"t,omitempty"`
}

func (c *conn) Backup(ctx context.Context, scope string, w io.Writer, p driver.Progress) (driver.BackupStats, error) {
	var st driver.BackupStats
	idx := dbIndex(scope)
	cl := c.client(idx)
	if idx < 0 {
		idx = c.opts.DB
	}
	bw := bufio.NewWriterSize(w, 1<<16)
	enc := json.NewEncoder(bw)
	_ = enc.Encode(map[string]any{"quarry": "redis-backup", "version": 1, "db": idx})

	iter := cl.Scan(ctx, 0, "*", 1000).Iterator()
	for iter.Next(ctx) {
		k := iter.Val()
		dump, err := cl.Dump(ctx, k).Result()
		if errors.Is(err, goredis.Nil) {
			continue // expired meanwhile
		}
		if err != nil {
			return st, fmt.Errorf("DUMP %s: %w", k, err)
		}
		e := entry{K: base64.StdEncoding.EncodeToString([]byte(k)), V: base64.StdEncoding.EncodeToString([]byte(dump))}
		if ttl, err := cl.PTTL(ctx, k).Result(); err == nil && ttl > 0 {
			e.T = ttl.Milliseconds()
		}
		if err := enc.Encode(e); err != nil {
			return st, err
		}
		st.Objects++
		st.Rows++
		if st.Rows%2000 == 0 {
			p(fmt.Sprintf("%d keys", st.Rows))
		}
	}
	if err := iter.Err(); err != nil {
		return st, err
	}
	return st, bw.Flush()
}

func (c *conn) Restore(ctx context.Context, scope string, r io.Reader, p driver.Progress) (driver.BackupStats, error) {
	var st driver.BackupStats
	cl := c.client(dbIndex(scope))
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 512<<20)
	if !sc.Scan() {
		return st, errors.New("empty backup file")
	}
	var h struct {
		Quarry string `json:"quarry"`
	}
	if json.Unmarshal(sc.Bytes(), &h) != nil || h.Quarry != "redis-backup" {
		return st, errors.New("this isn't a Quarry Redis backup")
	}
	pipe := cl.Pipeline()
	pending := 0
	exec := func() error {
		if pending == 0 {
			return nil
		}
		_, err := pipe.Exec(ctx)
		pending = 0
		p(fmt.Sprintf("%d keys", st.Rows))
		return err
	}
	for sc.Scan() {
		var e entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return st, err
		}
		k, err1 := base64.StdEncoding.DecodeString(e.K)
		v, err2 := base64.StdEncoding.DecodeString(e.V)
		if err := errors.Join(err1, err2); err != nil {
			return st, err
		}
		pipe.RestoreReplace(ctx, string(k), time.Duration(e.T)*time.Millisecond, string(v))
		pending++
		st.Rows++
		st.Objects++
		if pending >= 500 {
			if err := exec(); err != nil {
				return st, err
			}
		}
	}
	if err := sc.Err(); err != nil {
		return st, err
	}
	return st, exec()
}
