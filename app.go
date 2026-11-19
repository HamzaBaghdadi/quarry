package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/guard"
	"github.com/HamzaBaghdadi/quarry/internal/session"
	"github.com/HamzaBaghdadi/quarry/internal/store"
)

// App is the facade the frontend talks to. Every exported method becomes a
// typed TypeScript function via Wails bindings; keep them thin and push logic
// into internal packages.
type App struct {
	ctx      context.Context
	store    *store.Store
	sessions *session.Manager
}

func NewApp(s *store.Store) *App {
	return &App{store: s, sessions: session.NewManager()}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) shutdown(context.Context) { a.sessions.CloseAll() }

// Version is shown in the About panel.
func (a *App) Version() string { return version }

// Drivers lists every database engine Quarry can talk to.
func (a *App) Drivers() []driver.Info { return driver.All() }

// Connection is a saved profile plus live state.
type Connection struct {
	store.Profile
	Connected bool `json:"connected"`
}

func (a *App) ListConnections() ([]Connection, error) {
	ps, err := a.store.List()
	if err != nil {
		return nil, err
	}
	out := make([]Connection, len(ps))
	for i, p := range ps {
		out[i] = Connection{Profile: p, Connected: a.sessions.IsOpen(p.ID)}
	}
	return out, nil
}

// SaveConnection creates or updates a profile. When keepPassword is true the
// stored secret is left untouched.
func (a *App) SaveConnection(p store.Profile, keepPassword bool) (store.Profile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return store.Profile{}, errors.New("give the connection a name")
	}
	if _, err := driver.Get(p.Driver); err != nil {
		return store.Profile{}, err
	}
	if p.ID != "" {
		_ = a.sessions.Disconnect(p.ID) // settings changed; reconnect on next use
	}
	return a.store.Save(p, keepPassword)
}

func (a *App) DeleteConnection(id string) error {
	_ = a.sessions.Disconnect(id)
	return a.store.Delete(id)
}

// TestConnection dials without saving. If useStored is set and the password
// is empty, the saved password for p.ID is used.
func (a *App) TestConnection(p store.Profile, useStored bool) (string, error) {
	if useStored && p.ID != "" && p.Config.Password == "" {
		if saved, err := a.store.Get(p.ID); err == nil {
			p.Config.Password = saved.Config.Password
		}
	}
	start := time.Now()
	conn, err := session.Dial(a.ctx, p.Driver, p.Config)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	return fmt.Sprintf("Connected in %d ms", time.Since(start).Milliseconds()), nil
}

func (a *App) Connect(id string) error {
	p, err := a.store.Get(id)
	if err != nil {
		return err
	}
	return a.sessions.Connect(a.ctx, id, p.Driver, p.Config)
}

func (a *App) Disconnect(id string) error { return a.sessions.Disconnect(id) }

// Children lists explorer nodes beneath path.
func (a *App) Children(id string, path []string) ([]driver.Node, error) {
	c, err := a.sessions.Get(id)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	nodes, err := c.Children(ctx, path)
	if nodes == nil {
		nodes = []driver.Node{}
	}
	return nodes, err
}

// Browse reads a page of rows from a table, collection or key.
func (a *App) Browse(id string, req driver.BrowseRequest, runID string) (*driver.Result, error) {
	c, err := a.sessions.Get(id)
	if err != nil {
		return nil, err
	}
	ctx, done := a.sessions.Track(a.ctx, runID)
	defer done()
	return c.Browse(ctx, req)
}

// Execute runs a query in the engine's native language.
func (a *App) Execute(id, scope, query, runID string) (*driver.Result, error) {
	p, err := a.store.Get(id)
	if err != nil {
		return nil, err
	}
	if p.ReadOnly {
		d, _ := driver.Get(p.Driver)
		if err := guard.ReadOnly(d.Info().Language, query); err != nil {
			return nil, err
		}
	}
	c, err := a.sessions.Get(id)
	if err != nil {
		return nil, err
	}
	ctx, done := a.sessions.Track(a.ctx, runID)
	defer done()
	res, err := c.Execute(ctx, scope, query)
	if errors.Is(ctx.Err(), context.Canceled) {
		return nil, errors.New("query cancelled")
	}
	return res, err
}

// Cancel aborts an in-flight Browse/Execute.
func (a *App) Cancel(runID string) bool { return a.sessions.Cancel(runID) }

// PickFile opens a native file picker (used for SQLite).
func (a *App) PickFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose a database file",
		Filters: []runtime.FileFilter{
			{DisplayName: "SQLite databases", Pattern: "*.db;*.sqlite;*.sqlite3;*.db3"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
}

// Export writes a result set to CSV or JSON via a native save dialog.
// Returns the chosen path ("" if the user cancelled).
func (a *App) Export(format string, res driver.Result, suggestedName string) (string, error) {
	ext := map[string]string{"csv": "csv", "json": "json"}[format]
	if ext == "" {
		return "", fmt.Errorf("unknown export format %q", format)
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export results",
		DefaultFilename: suggestedName + "." + ext,
		Filters:         []runtime.FileFilter{{DisplayName: strings.ToUpper(ext), Pattern: "*." + ext}},
	})
	if err != nil || path == "" {
		return "", err
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	switch format {
	case "csv":
		w := csv.NewWriter(f)
		head := make([]string, len(res.Columns))
		for i, c := range res.Columns {
			head[i] = c.Name
		}
		_ = w.Write(head)
		for _, row := range res.Rows {
			rec := make([]string, len(row))
			for i, v := range row {
				if v != nil {
					rec[i] = fmt.Sprint(v)
				}
			}
			_ = w.Write(rec)
		}
		w.Flush()
		return path, w.Error()
	default:
		objs := make([]map[string]any, len(res.Rows))
		for i, row := range res.Rows {
			o := make(map[string]any, len(row))
			for j, v := range row {
				o[res.Columns[j].Name] = v
			}
			objs[i] = o
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		return path, enc.Encode(objs)
	}
}
