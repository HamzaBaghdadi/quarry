package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/HamzaBaghdadi/quarry/internal/driver"
	"github.com/HamzaBaghdadi/quarry/internal/guard"
	"github.com/HamzaBaghdadi/quarry/internal/session"
	"github.com/HamzaBaghdadi/quarry/internal/store"
	"github.com/HamzaBaghdadi/quarry/internal/updater"
)

// App is the facade the frontend talks to. Every exported method becomes a
// typed TypeScript function via Wails bindings; keep them thin and push logic
// into internal packages.
type App struct {
	ctx      context.Context
	store    *store.Store
	sessions *session.Manager

	mu      sync.Mutex
	pending *updater.Release
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

// DuplicateConnection copies a saved profile (including its stored password).
func (a *App) DuplicateConnection(id string) (store.Profile, error) {
	p, err := a.store.Get(id)
	if err != nil {
		return store.Profile{}, err
	}
	p.ID = ""
	p.Name += " (copy)"
	return a.store.Save(p, false)
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
	res, err := c.Browse(ctx, req)
	if err != nil {
		return nil, err
	}
	// Tell the grid whether (and how) rows can be edited.
	p, _ := a.store.Get(id)
	switch ed, ok := c.(driver.Editor); {
	case p.ReadOnly:
		res.EditBlocked = "This connection is read-only."
	case !ok:
		res.EditBlocked = "Editing isn't supported for this database."
	default:
		keys, err := ed.Keys(ctx, req.Path)
		if err != nil || len(keys) == 0 {
			res.EditBlocked = "No primary key — rows can't be identified safely, so this table is read-only."
		} else {
			res.Keys = keys
		}
	}
	return res, nil
}

// ApplyChanges saves edited, inserted and deleted rows.
func (a *App) ApplyChanges(id string, cs driver.ChangeSet) (*driver.Result, error) {
	p, err := a.store.Get(id)
	if err != nil {
		return nil, err
	}
	if p.ReadOnly {
		return nil, errors.New("this connection is read-only — edit it to allow writes")
	}
	c, err := a.sessions.Get(id)
	if err != nil {
		return nil, err
	}
	ed, ok := c.(driver.Editor)
	if !ok {
		return nil, errors.New("editing isn't supported for this database")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	return ed.Apply(ctx, cs)
}

// BackupReport is returned by Backup and Restore.
type BackupReport struct {
	Path       string             `json:"path"`
	Bytes      int64              `json:"bytes"`
	Stats      driver.BackupStats `json:"stats"`
	DurationMs int64              `json:"durationMs"`
}

// progress emits "progress" events the UI listens to.
func (a *App) progress(runID string, total int64, read func() int64) driver.Progress {
	last := time.Time{}
	return func(msg string) {
		if time.Since(last) < 100*time.Millisecond {
			return
		}
		last = time.Now()
		runtime.EventsEmit(a.ctx, "progress", map[string]any{"runId": runID, "message": msg, "done": read(), "total": total})
	}
}

func (a *App) backuper(id string) (driver.Backuper, store.Profile, error) {
	p, err := a.store.Get(id)
	if err != nil {
		return nil, p, err
	}
	c, err := a.sessions.Get(id)
	if err != nil {
		return nil, p, err
	}
	d, err := driver.Get(p.Driver)
	if err != nil {
		return nil, p, err
	}
	b, ok := c.(driver.Backuper)
	if !ok || !slices.Contains(d.Info().Capabilities, driver.CapBackup) {
		return nil, p, fmt.Errorf("built-in backups aren't available for %s yet — use its native tools", d.Info().Name)
	}
	return b, p, nil
}

// Backup dumps database scope to a file the user picks. Returns a zero
// report if the user cancels the save dialog.
func (a *App) Backup(id, scope, runID string) (*BackupReport, error) {
	b, p, err := a.backuper(id)
	if err != nil {
		return nil, err
	}
	name := strings.Trim(nonWord.ReplaceAllString(p.Name+"-"+scope, "-"), "-")
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save backup",
		DefaultFilename: fmt.Sprintf("%s-%s.%s", name, time.Now().Format("20060102-1504"), b.BackupFormat()),
		Filters:         []runtime.FileFilter{{DisplayName: "Quarry backup", Pattern: "*." + b.BackupFormat()}},
	})
	if err != nil || path == "" {
		return &BackupReport{}, err
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	cw := &countingWriter{w: f}
	ctx, done := a.sessions.Track(a.ctx, runID)
	defer done()
	start := time.Now()
	stats, err := b.Backup(ctx, scope, cw, a.progress(runID, 0, func() int64 { return cw.n }))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(path) // never leave a truncated backup behind
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, errors.New("backup cancelled")
		}
		return nil, err
	}
	return &BackupReport{Path: path, Bytes: cw.n, Stats: stats, DurationMs: time.Since(start).Milliseconds()}, nil
}

// PickBackupFile lets the user choose a backup to restore ("" if cancelled).
func (a *App) PickBackupFile(id string) (string, error) {
	b, _, err := a.backuper(id)
	if err != nil {
		return "", err
	}
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Choose a backup to restore",
		Filters: []runtime.FileFilter{{DisplayName: "Quarry backup", Pattern: "*." + b.BackupFormat()}, {DisplayName: "All files", Pattern: "*.*"}},
	})
}

// Restore loads a backup file into database scope.
func (a *App) Restore(id, scope, path, runID string) (*BackupReport, error) {
	b, p, err := a.backuper(id)
	if err != nil {
		return nil, err
	}
	if p.ReadOnly {
		return nil, errors.New("this connection is read-only — edit it to allow restores")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, _ := f.Stat()
	cr := &countingReader{r: f}
	ctx, done := a.sessions.Track(a.ctx, runID)
	defer done()
	start := time.Now()
	stats, err := b.Restore(ctx, scope, cr, a.progress(runID, info.Size(), func() int64 { return cr.n }))
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, errors.New("restore cancelled — changes made before cancelling may remain unless the backup ran in a transaction")
		}
		return nil, err
	}
	return &BackupReport{Path: path, Bytes: cr.n, Stats: stats, DurationMs: time.Since(start).Milliseconds()}, nil
}

var nonWord = regexp.MustCompile(`[^\w.-]+`)

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
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

// --- Updates -------------------------------------------------------------

// CheckForUpdate returns the newer release, or nil if Quarry is current.
func (a *App) CheckForUpdate() (*updater.Release, error) {
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	a.mu.Lock()
	defer a.mu.Unlock()
	rel, err := updater.Check(ctx, version)
	if err != nil {
		return nil, err
	}
	a.pending = rel
	return rel, nil
}

// InstallUpdate downloads and installs the release found by CheckForUpdate,
// emitting "update:progress" events. Returns "restart" when the app should
// relaunch itself, or "installer" when an installer took over.
func (a *App) InstallUpdate() (string, error) {
	a.mu.Lock()
	rel := a.pending
	a.mu.Unlock()
	if rel == nil {
		return "", errors.New("no update to install — check again")
	}
	mode, err := updater.Apply(a.ctx, rel, func(done, total int64) {
		runtime.EventsEmit(a.ctx, "update:progress", map[string]int64{"done": done, "total": total})
	})
	if err != nil {
		return "", err
	}
	switch mode {
	case "installer": // Windows installer takes over and reopens Quarry
		runtime.Quit(a.ctx)
		return "installer", nil
	case "package": // Linux software installer opened; the user restarts when done
		return "package", nil
	}
	return "restart", nil
}

// RestartApp relaunches Quarry (after an update).
func (a *App) RestartApp() error {
	if err := updater.Restart(); err != nil {
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}
