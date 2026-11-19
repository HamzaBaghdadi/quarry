package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Window placement: the window starts hidden, is sized to fit the screen it
// opens on (remembering the user's last size), centred, then shown — so it
// never opens partly off-screen on small or high-DPI displays.

const (
	defaultW = 1360
	defaultH = 860
	minW     = 900
	minH     = 560
)

type windowState struct {
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised"`
}

func windowStatePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Quarry", "window.json")
}

func loadWindowState() windowState {
	st := windowState{Width: defaultW, Height: defaultH}
	if p := windowStatePath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			_ = json.Unmarshal(b, &st)
		}
	}
	return st
}

// placeWindow fits the window to the current screen and shows it.
func (a *App) placeWindow(ctx context.Context) {
	st := loadWindowState()
	w, h := st.Width, st.Height
	if screens, err := runtime.ScreenGetAll(ctx); err == nil && len(screens) > 0 {
		s := screens[0]
		for _, sc := range screens {
			if sc.IsCurrent || (sc.IsPrimary && !s.IsCurrent) {
				s = sc
			}
		}
		// Leave room for the taskbar / dock and a small margin.
		maxW, maxH := s.Size.Width-48, s.Size.Height-96
		if w > maxW {
			w = maxW
		}
		if h > maxH {
			h = maxH
		}
		// Small screens: just maximise.
		if maxW < minW || maxH < minH {
			st.Maximised = true
		}
	}
	w, h = max(w, minW), max(h, minH)
	runtime.WindowSetSize(ctx, w, h)
	runtime.WindowCenter(ctx)
	if st.Maximised {
		runtime.WindowMaximise(ctx)
	}
	runtime.WindowShow(ctx)
}

// saveWindowState remembers the size for next launch.
func (a *App) saveWindowState(ctx context.Context) bool {
	st := windowState{Maximised: runtime.WindowIsMaximised(ctx)}
	if st.Maximised {
		prev := loadWindowState()
		st.Width, st.Height = prev.Width, prev.Height
	} else {
		st.Width, st.Height = runtime.WindowGetSize(ctx)
	}
	if p := windowStatePath(); p != "" && st.Width >= minW && st.Height >= minH {
		_ = os.MkdirAll(filepath.Dir(p), 0o700)
		if b, err := json.Marshal(st); err == nil {
			_ = os.WriteFile(p, b, 0o600)
		}
	}
	return false // don't prevent closing
}

// CloseWindow saves the window size and quits (used by the custom title bar).
func (a *App) CloseWindow() {
	a.saveWindowState(a.ctx)
	runtime.Quit(a.ctx)
}
