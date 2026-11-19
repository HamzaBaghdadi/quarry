// Package redis adapts Redis and protocol-compatible stores (Valkey, KeyDB, Dragonfly).
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"

	goredis "github.com/redis/go-redis/v9"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

func init() {
	for _, info := range []driver.Info{{ID: "redis", Name: "Redis"}, {ID: "valkey", Name: "Valkey"}} {
		info.Family, info.Language, info.DefaultPort = driver.FamilyKeyValue, "redis", 6379
		info.Fields = []driver.Field{driver.FieldHost, driver.FieldPort, driver.FieldUser, driver.FieldPassword, driver.FieldDatabase, driver.FieldSSL, driver.FieldURI}
		info.SSLModes = []string{"false", "true"}
		info.URIPlaceholder = "redis://:password@localhost:6379/0"
		info.Example = "INFO server"
		info.Capabilities = []string{driver.CapEdit, driver.CapBackup}
		driver.Register(&Driver{info: info})
	}
}

type Driver struct{ info driver.Info }

func (d *Driver) Info() driver.Info { return d.info }

func (d *Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	var opts *goredis.Options
	if uri := strings.TrimSpace(cfg.URI); uri != "" {
		o, err := goredis.ParseURL(uri)
		if err != nil {
			return nil, fmt.Errorf("invalid URL: %w", err)
		}
		opts = o
	} else {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 6379
		}
		db, _ := strconv.Atoi(cfg.Database)
		opts = &goredis.Options{Addr: net.JoinHostPort(host, strconv.Itoa(port)), Username: cfg.User, Password: cfg.Password, DB: db}
		if cfg.SSLMode == "true" {
			opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}
		}
	}
	opts.ClientName = "quarry"
	c := &conn{opts: *opts, clients: map[int]*goredis.Client{}}
	if err := c.Ping(ctx); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

type conn struct {
	opts    goredis.Options
	mu      sync.Mutex
	clients map[int]*goredis.Client
}

// client returns a client bound to logical database idx (-1 = configured default).
func (c *conn) client(idx int) *goredis.Client {
	if idx < 0 {
		idx = c.opts.DB
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cl, ok := c.clients[idx]; ok {
		return cl
	}
	o := c.opts
	o.DB = idx
	o.PoolSize = 4
	cl := goredis.NewClient(&o)
	c.clients[idx] = cl
	return cl
}

func (c *conn) Ping(ctx context.Context) error { return c.client(-1).Ping(ctx).Err() }

func (c *conn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	for _, cl := range c.clients {
		errs = append(errs, cl.Close())
	}
	return errors.Join(errs...)
}

func dbIndex(s string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(s, "db"))
	if err != nil {
		return -1
	}
	return n
}

const maxKeys = 2000

func (c *conn) Children(ctx context.Context, path []string) ([]driver.Node, error) {
	switch len(path) {
	case 0:
		count := 16
		if v, err := c.client(-1).ConfigGet(ctx, "databases").Result(); err == nil {
			if n, err := strconv.Atoi(v["databases"]); err == nil && n > 0 {
				count = n
			}
		}
		keyspace := map[int]string{}
		if info, err := c.client(-1).Info(ctx, "keyspace").Result(); err == nil {
			for _, line := range strings.Split(info, "\n") {
				if strings.HasPrefix(line, "db") {
					name, rest, _ := strings.Cut(strings.TrimSpace(line), ":")
					keys, _, _ := strings.Cut(strings.TrimPrefix(rest, "keys="), ",")
					keyspace[dbIndex(name)] = keys + " keys"
				}
			}
		}
		nodes := []driver.Node{}
		for i := 0; i < count; i++ {
			detail, used := keyspace[i]
			// Only list empty databases up to 16 to keep the tree tidy.
			if !used && i >= 16 {
				continue
			}
			nodes = append(nodes, driver.Node{Name: fmt.Sprintf("db%d", i), Kind: driver.KindDatabase,
				Path: []string{fmt.Sprintf("db%d", i)}, HasKids: used, Detail: detail})
		}
		return nodes, nil
	case 1:
		cl := c.client(dbIndex(path[0]))
		var keys []string
		iter := cl.Scan(ctx, 0, "*", 500).Iterator()
		for iter.Next(ctx) && len(keys) < maxKeys {
			keys = append(keys, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return nil, err
		}
		sort.Strings(keys)
		pipe := cl.Pipeline()
		types := make([]*goredis.StatusCmd, len(keys))
		for i, k := range keys {
			types[i] = pipe.Type(ctx, k)
		}
		_, _ = pipe.Exec(ctx)
		nodes := make([]driver.Node, len(keys))
		for i, k := range keys {
			nodes[i] = driver.Node{Name: k, Kind: driver.KindKey, Path: []string{path[0], k}, Browsable: true, Detail: types[i].Val()}
		}
		return nodes, nil
	}
	return nil, nil
}

func (c *conn) Browse(ctx context.Context, req driver.BrowseRequest) (*driver.Result, error) {
	t := driver.StartTimer()
	if len(req.Path) != 2 {
		return nil, errors.New("pick a key")
	}
	cl := c.client(dbIndex(req.Path[0]))
	key := req.Path[1]
	limit := int64(req.Limit)
	if limit <= 0 {
		limit = 200
	}
	start, stop := int64(req.Offset), int64(req.Offset)+limit
	typ, err := cl.Type(ctx, key).Result()
	if err != nil {
		return nil, err
	}
	r := &driver.Result{Rows: [][]any{}}
	switch typ {
	case "none":
		return nil, fmt.Errorf("key %q no longer exists", key)
	case "string":
		v, err := cl.Get(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		r.Columns = []driver.Column{{Name: "value", Type: "string"}}
		r.Rows = append(r.Rows, []any{v})
	case "list":
		vals, err := cl.LRange(ctx, key, start, stop).Result()
		if err != nil {
			return nil, err
		}
		r.Columns = []driver.Column{{Name: "index", Type: "int"}, {Name: "value", Type: "string"}}
		for i, v := range vals {
			r.Rows = append(r.Rows, []any{start + int64(i), v})
		}
	case "hash":
		m, err := cl.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		fields := make([]string, 0, len(m))
		for f := range m {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		r.Columns = []driver.Column{{Name: "field", Type: "string"}, {Name: "value", Type: "string"}}
		for _, f := range fields {
			r.Rows = append(r.Rows, []any{f, m[f]})
		}
	case "set":
		vals, err := cl.SMembers(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		sort.Strings(vals)
		r.Columns = []driver.Column{{Name: "member", Type: "string"}}
		for _, v := range vals {
			r.Rows = append(r.Rows, []any{v})
		}
	case "zset":
		vals, err := cl.ZRangeWithScores(ctx, key, start, stop).Result()
		if err != nil {
			return nil, err
		}
		r.Columns = []driver.Column{{Name: "member", Type: "string"}, {Name: "score", Type: "double"}}
		for _, z := range vals {
			r.Rows = append(r.Rows, []any{fmt.Sprint(z.Member), z.Score})
		}
	case "stream":
		msgs, err := cl.XRangeN(ctx, key, "-", "+", stop).Result()
		if err != nil {
			return nil, err
		}
		r.Columns = []driver.Column{{Name: "id", Type: "string"}, {Name: "fields", Type: "object"}}
		for _, m := range msgs {
			r.Rows = append(r.Rows, []any{m.ID, driver.Normalize(m.Values)})
		}
	default:
		return nil, fmt.Errorf("unsupported key type %q", typ)
	}
	if len(r.Rows) > int(limit) {
		r.Rows, r.HasMore = r.Rows[:limit], true
	}
	if ttl, err := cl.TTL(ctx, key).Result(); err == nil && ttl > 0 {
		r.Message = fmt.Sprintf("%s · TTL %s", typ, ttl)
	} else {
		r.Message = typ
	}
	r.Statement = fmt.Sprintf("TYPE %s", quote(key))
	return t.Stamp(r), nil
}

func (c *conn) Execute(ctx context.Context, scope, query string) (*driver.Result, error) {
	t := driver.StartTimer()
	cl := c.client(dbIndex(scope))
	out := &driver.Result{Rows: [][]any{}}
	// One command per line, like redis-cli.
	for _, line := range strings.Split(query, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		parts, err := split(line)
		if err != nil {
			return nil, err
		}
		if strings.EqualFold(parts[0], "select") {
			return nil, errors.New("pick the database from the scope selector instead of SELECT")
		}
		args := make([]any, len(parts))
		for i, p := range parts {
			args[i] = p
		}
		v, err := cl.Do(ctx, args...).Result()
		if err != nil && !errors.Is(err, goredis.Nil) {
			return nil, fmt.Errorf("%s: %w", parts[0], err)
		}
		out = reply(v)
		out.Statement = line
	}
	return t.Stamp(out), nil
}

// reply shapes a RESP reply as a grid.
func reply(v any) *driver.Result {
	r := &driver.Result{Rows: [][]any{}}
	switch x := v.(type) {
	case nil:
		r.Columns = []driver.Column{{Name: "reply"}}
		r.Rows = append(r.Rows, []any{nil})
		r.Message = "(nil)"
	case []any:
		r.Columns = []driver.Column{{Name: "#", Type: "int"}, {Name: "value"}}
		for i, e := range x {
			r.Rows = append(r.Rows, []any{i + 1, driver.Normalize(e)})
		}
	case map[any]any:
		r.Columns = []driver.Column{{Name: "field"}, {Name: "value"}}
		for k, e := range x {
			r.Rows = append(r.Rows, []any{fmt.Sprint(k), driver.Normalize(e)})
		}
	case string:
		// INFO-style multi-line "key:value" replies read better as a table.
		if strings.Contains(x, "\r\n") && strings.Contains(x, ":") {
			r.Columns = []driver.Column{{Name: "key"}, {Name: "value"}}
			for _, line := range strings.Split(x, "\r\n") {
				if k, val, ok := strings.Cut(line, ":"); ok && !strings.HasPrefix(line, "#") {
					r.Rows = append(r.Rows, []any{k, val})
				}
			}
			return r
		}
		r.Columns = []driver.Column{{Name: "reply"}}
		r.Rows = append(r.Rows, []any{x})
	default:
		r.Columns = []driver.Column{{Name: "reply"}}
		r.Rows = append(r.Rows, []any{driver.Normalize(x)})
	}
	return r
}

// split tokenizes a redis-cli style line, honouring quotes.
func split(line string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	inToken := false
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, inToken = r, true
		case unicode.IsSpace(r):
			if inToken {
				out = append(out, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inToken {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return nil, errors.New("empty command")
	}
	return out, nil
}

func quote(s string) string {
	if strings.ContainsAny(s, " \"'") {
		return strconv.Quote(s)
	}
	return s
}
