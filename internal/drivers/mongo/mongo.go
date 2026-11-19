// Package mongo adapts MongoDB (and API-compatible services such as
// DocumentDB, Cosmos DB for MongoDB and FerretDB).
package mongo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
)

func init() { driver.Register(&Driver{}) }

type Driver struct{}

func (Driver) Info() driver.Info {
	return driver.Info{
		ID: "mongodb", Name: "MongoDB", Family: driver.FamilyDocument, Language: "mongodb",
		DefaultPort: 27017,
		Fields:         []driver.Field{driver.FieldHost, driver.FieldPort, driver.FieldUser, driver.FieldPassword, driver.FieldDatabase, driver.FieldSSL, driver.FieldURI},
		SSLModes:       []string{"false", "true"},
		URIPlaceholder: "mongodb+srv://user:pass@cluster.example.net/app",
		Example:        "db.getCollection(\"users\").find({}).limit(20)",
		Capabilities:   []string{driver.CapEdit, driver.CapBackup},
	}
}

func (Driver) Open(ctx context.Context, cfg driver.Config) (driver.Conn, error) {
	uri := strings.TrimSpace(cfg.URI)
	if uri == "" {
		host := cfg.Host
		if host == "" {
			host = "localhost"
		}
		port := cfg.Port
		if port == 0 {
			port = 27017
		}
		u := url.URL{Scheme: "mongodb", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/" + cfg.Database}
		if cfg.User != "" {
			u.User = url.UserPassword(cfg.User, cfg.Password)
		}
		q := url.Values{}
		if cfg.SSLMode == "true" {
			q.Set("tls", "true")
		}
		if cfg.Database != "" && cfg.User != "" {
			q.Set("authSource", "admin")
		}
		u.RawQuery = q.Encode()
		uri = u.String()
	}
	opts := options.Client().ApplyURI(uri).SetAppName("Quarry")
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	def := cfg.Database
	if def == "" {
		if cs, err := url.Parse(uri); err == nil {
			def = strings.TrimPrefix(cs.Path, "/")
		}
	}
	return &conn{client: client, defaultDB: def}, nil
}

type conn struct {
	client    *mongo.Client
	defaultDB string
}

func (c *conn) Ping(ctx context.Context) error { return c.client.Ping(ctx, nil) }
func (c *conn) Close() error                   { return c.client.Disconnect(context.Background()) }

func (c *conn) Children(ctx context.Context, path []string) ([]driver.Node, error) {
	switch len(path) {
	case 0:
		names, err := c.client.ListDatabaseNames(ctx, bson.D{})
		if err != nil {
			// Users scoped to one database often can't list all of them.
			if c.defaultDB != "" {
				names = []string{c.defaultDB}
			} else {
				return nil, err
			}
		}
		nodes := make([]driver.Node, 0, len(names))
		for _, n := range names {
			nodes = append(nodes, driver.Node{Name: n, Kind: driver.KindDatabase, Path: []string{n}, HasKids: true})
		}
		return nodes, nil
	case 1:
		specs, err := c.client.Database(path[0]).ListCollectionSpecifications(ctx, bson.D{})
		if err != nil {
			return nil, err
		}
		sort.Slice(specs, func(i, j int) bool { return specs[i].Name < specs[j].Name })
		nodes := make([]driver.Node, 0, len(specs))
		for _, s := range specs {
			kind := driver.KindCollection
			if s.Type == "view" {
				kind = driver.KindView
			}
			nodes = append(nodes, driver.Node{Name: s.Name, Kind: kind, Path: []string{path[0], s.Name}, Browsable: true})
		}
		return nodes, nil
	}
	return nil, nil
}

func (c *conn) Browse(ctx context.Context, req driver.BrowseRequest) (*driver.Result, error) {
	t := driver.StartTimer()
	if len(req.Path) != 2 {
		return nil, errors.New("pick a collection")
	}
	if req.Limit <= 0 {
		req.Limit = 200
	}
	filter := bson.D{}
	if f := strings.TrimSpace(req.Filter); f != "" {
		js, err := toJSON(f)
		if err != nil {
			return nil, err
		}
		if err := bson.UnmarshalExtJSON([]byte(js), false, &filter); err != nil {
			return nil, fmt.Errorf("filter: %w", err)
		}
	}
	fo := options.Find().SetLimit(int64(req.Limit + 1)).SetSkip(int64(req.Offset))
	if req.OrderBy != "" {
		dir := 1
		if req.Desc {
			dir = -1
		}
		fo.SetSort(bson.D{{Key: req.OrderBy, Value: dir}})
	}
	cur, err := c.client.Database(req.Path[0]).Collection(req.Path[1]).Find(ctx, filter, fo)
	if err != nil {
		return nil, err
	}
	res, err := documents(ctx, cur, req.Limit)
	if err != nil {
		return nil, err
	}
	stmt, _ := bson.MarshalExtJSON(filter, false, false)
	res.Statement = fmt.Sprintf("db.getCollection(%q).find(%s)", req.Path[1], stmt)
	return t.Stamp(res), nil
}

const maxDocs = 5000

func (c *conn) Execute(ctx context.Context, scope, query string) (*driver.Result, error) {
	t := driver.StartTimer()
	dbName := scope
	if dbName == "" {
		dbName = c.defaultDB
	}
	if dbName == "" {
		dbName = "test"
	}
	db := c.client.Database(dbName)
	q := strings.TrimSpace(query)

	switch strings.ToLower(strings.TrimSuffix(q, ";")) {
	case "show dbs", "show databases":
		nodes, err := c.Children(ctx, nil)
		return t.Stamp(nameList("database", nodes)), err
	case "show collections":
		nodes, err := c.Children(ctx, []string{dbName})
		return t.Stamp(nameList("collection", nodes)), err
	}

	// Raw database command: { "ping": 1 }
	if strings.HasPrefix(q, "{") {
		cmd, err := parseDoc(q)
		if err != nil {
			return nil, err
		}
		var out bson.D
		if err := db.RunCommand(ctx, cmd).Decode(&out); err != nil {
			return nil, err
		}
		return t.Stamp(fromDocs([]bson.D{out})), nil
	}

	expr, err := parseShell(q)
	if err != nil {
		return nil, err
	}
	res, err := run(ctx, db.Collection(expr.Collection), expr)
	if err != nil {
		return nil, err
	}
	return t.Stamp(res), nil
}

func nameList(col string, nodes []driver.Node) *driver.Result {
	r := &driver.Result{Columns: []driver.Column{{Name: col, Type: "string"}}, Rows: [][]any{}}
	for _, n := range nodes {
		r.Rows = append(r.Rows, []any{n.Name})
	}
	return r
}

// args parses a call's argument list into BSON values.
func args(src string) ([]any, error) {
	if strings.TrimSpace(src) == "" {
		return nil, nil
	}
	js, err := toJSON("{\"a\":[" + src + "]}")
	if err != nil {
		return nil, err
	}
	var wrap struct {
		A bson.A `bson:"a"`
	}
	if err := bson.UnmarshalExtJSON([]byte(js), false, &wrap); err != nil {
		return nil, fmt.Errorf("could not parse arguments: %w", err)
	}
	return wrap.A, nil
}

func parseDoc(src string) (bson.D, error) {
	js, err := toJSON(src)
	if err != nil {
		return nil, err
	}
	var d bson.D
	if err := bson.UnmarshalExtJSON([]byte(js), false, &d); err != nil {
		return nil, err
	}
	return d, nil
}

func arg(a []any, i int) any {
	if i < len(a) {
		return a[i]
	}
	return bson.D{}
}

func run(ctx context.Context, coll *mongo.Collection, e *shellExpr) (*driver.Result, error) {
	head := e.Calls[0]
	a, err := args(head.Args)
	if err != nil {
		return nil, err
	}
	// Collect cursor modifiers from the chain.
	var sortSpec any
	var limit, skip int64 = maxDocs, 0
	var projection any
	for _, ch := range e.Calls[1:] {
		ca, err := args(ch.Args)
		if err != nil {
			return nil, err
		}
		switch ch.Name {
		case "sort":
			sortSpec = arg(ca, 0)
		case "limit":
			limit = toInt(arg(ca, 0), maxDocs)
		case "skip":
			skip = toInt(arg(ca, 0), 0)
		case "projection", "project":
			projection = arg(ca, 0)
		case "pretty", "toArray", "count":
		default:
			return nil, fmt.Errorf("unsupported chained method .%s()", ch.Name)
		}
	}
	if limit <= 0 || limit > maxDocs {
		limit = maxDocs
	}

	switch head.Name {
	case "find":
		fo := options.Find().SetLimit(limit + 1).SetSkip(skip)
		if len(a) > 1 {
			fo.SetProjection(a[1])
		}
		if projection != nil {
			fo.SetProjection(projection)
		}
		if sortSpec != nil {
			fo.SetSort(sortSpec)
		}
		cur, err := coll.Find(ctx, arg(a, 0), fo)
		if err != nil {
			return nil, err
		}
		return documents(ctx, cur, int(limit))
	case "findOne":
		var d bson.D
		fo := options.FindOne()
		if len(a) > 1 {
			fo.SetProjection(a[1])
		}
		err := coll.FindOne(ctx, arg(a, 0), fo).Decode(&d)
		if errors.Is(err, mongo.ErrNoDocuments) {
			return &driver.Result{Message: "No document matched", Rows: [][]any{}}, nil
		}
		if err != nil {
			return nil, err
		}
		return fromDocs([]bson.D{d}), nil
	case "aggregate":
		cur, err := coll.Aggregate(ctx, arg(a, 0))
		if err != nil {
			return nil, err
		}
		return documents(ctx, cur, maxDocs)
	case "countDocuments", "count":
		n, err := coll.CountDocuments(ctx, arg(a, 0))
		return scalar("count", n), err
	case "estimatedDocumentCount":
		n, err := coll.EstimatedDocumentCount(ctx)
		return scalar("count", n), err
	case "distinct":
		field, _ := arg(a, 0).(string)
		var vals []any
		err := coll.Distinct(ctx, field, arg(a, 1)).Decode(&vals)
		if err != nil {
			return nil, err
		}
		r := &driver.Result{Columns: []driver.Column{{Name: field}}, Rows: [][]any{}}
		for _, v := range vals {
			r.Rows = append(r.Rows, []any{cell(v)})
		}
		return r, nil
	case "insertOne":
		r, err := coll.InsertOne(ctx, arg(a, 0))
		if err != nil {
			return nil, err
		}
		return &driver.Result{RowsAffected: 1, Message: fmt.Sprintf("Inserted 1 document (_id %v)", cell(r.InsertedID))}, nil
	case "insertMany":
		docs, _ := arg(a, 0).(bson.A)
		r, err := coll.InsertMany(ctx, docs)
		if err != nil {
			return nil, err
		}
		return affected(int64(len(r.InsertedIDs)), "Inserted"), nil
	case "updateOne", "updateMany", "replaceOne":
		var r *mongo.UpdateResult
		switch head.Name {
		case "updateOne":
			r, err = coll.UpdateOne(ctx, arg(a, 0), arg(a, 1))
		case "updateMany":
			r, err = coll.UpdateMany(ctx, arg(a, 0), arg(a, 1))
		default:
			r, err = coll.ReplaceOne(ctx, arg(a, 0), arg(a, 1))
		}
		if err != nil {
			return nil, err
		}
		return &driver.Result{RowsAffected: r.ModifiedCount,
			Message: fmt.Sprintf("Matched %d, modified %d, upserted %d", r.MatchedCount, r.ModifiedCount, r.UpsertedCount)}, nil
	case "deleteOne", "deleteMany":
		var r *mongo.DeleteResult
		if head.Name == "deleteOne" {
			r, err = coll.DeleteOne(ctx, arg(a, 0))
		} else {
			r, err = coll.DeleteMany(ctx, arg(a, 0))
		}
		if err != nil {
			return nil, err
		}
		return affected(r.DeletedCount, "Deleted"), nil
	case "getIndexes":
		cur, err := coll.Indexes().List(ctx)
		if err != nil {
			return nil, err
		}
		return documents(ctx, cur, maxDocs)
	case "createIndex":
		name, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: arg(a, 0)})
		if err != nil {
			return nil, err
		}
		return &driver.Result{Message: "Created index " + name}, nil
	case "drop":
		if err := coll.Drop(ctx); err != nil {
			return nil, err
		}
		return &driver.Result{Message: "Dropped collection " + coll.Name()}, nil
	}
	return nil, fmt.Errorf("unsupported method .%s()", head.Name)
}

func affected(n int64, verb string) *driver.Result {
	return &driver.Result{RowsAffected: n, Message: fmt.Sprintf("%s %d document(s)", verb, n)}
}

func scalar(name string, v any) *driver.Result {
	return &driver.Result{Columns: []driver.Column{{Name: name}}, Rows: [][]any{{v}}}
}

func toInt(v any, def int64) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return def
}

func documents(ctx context.Context, cur *mongo.Cursor, limit int) (*driver.Result, error) {
	defer cur.Close(ctx)
	var docs []bson.D
	more := false
	for cur.Next(ctx) {
		if len(docs) >= limit {
			more = true
			break
		}
		var d bson.D
		if err := cur.Decode(&d); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	r := fromDocs(docs)
	r.HasMore = more
	return r, nil
}

// fromDocs flattens documents into a grid (union of top-level fields, _id
// first) and keeps a lossless JSON copy of each one.
func fromDocs(docs []bson.D) *driver.Result {
	r := &driver.Result{Rows: make([][]any, 0, len(docs)), Documents: make([]string, 0, len(docs))}
	index := map[string]int{}
	types := map[string]string{}
	for _, d := range docs {
		for _, e := range d {
			if _, ok := index[e.Key]; !ok {
				index[e.Key] = len(r.Columns)
				r.Columns = append(r.Columns, driver.Column{Name: e.Key})
			}
			if types[e.Key] == "" && e.Value != nil {
				types[e.Key] = typeName(e.Value)
			}
		}
	}
	if i, ok := index["_id"]; ok && i != 0 {
		id := r.Columns[i]
		copy(r.Columns[1:i+1], r.Columns[:i])
		r.Columns[0] = id
		for k := range index {
			index[k] = -1
		}
		for j, c := range r.Columns {
			index[c.Name] = j
		}
	}
	for i := range r.Columns {
		r.Columns[i].Type = types[r.Columns[i].Name]
	}
	for _, d := range docs {
		row := make([]any, len(r.Columns))
		for _, e := range d {
			row[index[e.Key]] = cell(e.Value)
		}
		r.Rows = append(r.Rows, row)
		if js, err := bson.MarshalExtJSONIndent(d, false, false, "", "  "); err == nil {
			r.Documents = append(r.Documents, string(js))
		}
	}
	return r
}

func typeName(v any) string {
	switch v.(type) {
	case bson.ObjectID:
		return "objectId"
	case string:
		return "string"
	case int32, int64:
		return "int"
	case float64:
		return "double"
	case bool:
		return "bool"
	case bson.DateTime:
		return "date"
	case bson.D:
		return "object"
	case bson.A:
		return "array"
	case bson.Decimal128:
		return "decimal"
	}
	return fmt.Sprintf("%T", v)
}

// cell renders one BSON value for the grid.
func cell(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case bson.ObjectID:
		return x.Hex()
	case bson.DateTime:
		return x.Time().UTC().Format("2006-01-02T15:04:05.000Z")
	case bson.D, bson.A, bson.M:
		b, err := bson.MarshalExtJSON(bson.D{{Key: "v", Value: x}}, false, false)
		if err != nil {
			return fmt.Sprint(x)
		}
		var w struct {
			V json.RawMessage `json:"v"`
		}
		if json.Unmarshal(b, &w) == nil {
			return string(w.V)
		}
		return string(b)
	case bson.Decimal128:
		return x.String()
	case bson.Binary:
		return fmt.Sprintf("Binary(%d bytes)", len(x.Data))
	}
	return driver.Normalize(v)
}
