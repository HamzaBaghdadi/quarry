// Package updater keeps Quarry current using GitHub Releases.
//
// Each release carries one self-contained binary per platform plus a
// checksums.txt (sha256). Updating downloads the binary for this OS/arch,
// verifies its checksum, swaps it in place of the running executable and
// relaunches. If the install location isn't writable (e.g. Program Files),
// Windows falls back to running the release's installer.
package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository releases are published to.
const Repo = "HamzaBaghdadi/quarry"

// Release describes an available update.
type Release struct {
	Version     string `json:"version"` // without leading "v"
	Name        string `json:"name"`
	Notes       string `json:"notes"`
	URL         string `json:"url"` // release page
	PublishedAt string `json:"publishedAt"`
	Size        int64  `json:"size"`   // bytes that will be downloaded
	// Method is "replace" (swap the executable in place) or "installer"
	// (Windows, when the install folder isn't writable).
	Method string `json:"method"`

	asset     string // binary download URL
	installer string // Windows installer URL (optional)
	checksums string // checksums.txt URL
	assetName string
	instName  string
	instSize  int64
}

type ghAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt string    `json:"published_at"`
	Assets      []ghAsset `json:"assets"`
}

// AssetName is the release binary for this platform.
func AssetName() string {
	switch runtime.GOOS {
	case "windows":
		return "quarry-windows-" + runtime.GOARCH + ".exe"
	case "darwin":
		return "quarry-darwin-universal"
	default:
		if os.Getenv("APPIMAGE") != "" {
			return "quarry-linux-" + runtime.GOARCH + ".AppImage"
		}
		return "quarry-linux-" + runtime.GOARCH
	}
}

// target is the file an in-place update replaces: the AppImage itself when
// running from one, otherwise the executable.
func target() (string, error) {
	if p := os.Getenv("APPIMAGE"); runtime.GOOS == "linux" && p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(exe)
}

// linuxPackage reports which system package (".deb"/".rpm") Quarry was
// installed from, or "" for a self-managed copy (tarball / AppImage).
func linuxPackage() string {
	if runtime.GOOS != "linux" || os.Getenv("APPIMAGE") != "" {
		return ""
	}
	exe, err := target()
	if err != nil || !strings.HasPrefix(exe, "/usr/") {
		return ""
	}
	if _, err := os.Stat("/usr/bin/dpkg"); err == nil {
		return ".deb"
	}
	if _, err := os.Stat("/usr/bin/rpm"); err == nil {
		return ".rpm"
	}
	return ""
}

var client = &http.Client{Timeout: 30 * time.Second}

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Quarry-Updater")
	return client.Do(req)
}

// Check returns the latest release if it's newer than current, else nil.
func Check(ctx context.Context, current string) (*Release, error) {
	resp, err := get(ctx, "https://api.github.com/repos/"+Repo+"/releases/latest")
	if err != nil {
		return nil, fmt.Errorf("couldn't reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no releases yet
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub returned %s", resp.Status)
	}
	var gh ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return nil, err
	}
	latest := strings.TrimPrefix(gh.TagName, "v")
	if gh.Draft || gh.Prerelease || !Newer(latest, current) {
		return nil, nil
	}
	rel := &Release{Version: latest, Name: gh.Name, Notes: gh.Body, URL: gh.HTMLURL, PublishedAt: gh.PublishedAt}
	want := AssetName()
	pkg := linuxPackage()
	for _, a := range gh.Assets {
		switch {
		case a.Name == want:
			rel.asset, rel.assetName, rel.Size = a.URL, a.Name, a.Size
		case a.Name == "checksums.txt":
			rel.checksums = a.URL
		case runtime.GOOS == "windows" && strings.HasSuffix(a.Name, "-installer.exe"):
			rel.installer, rel.instName, rel.instSize = a.URL, a.Name, a.Size
		case runtime.GOOS == "linux" && pkg != "" && a.Name == "quarry-linux-"+runtime.GOARCH+pkg:
			rel.installer, rel.instName, rel.instSize = a.URL, a.Name, a.Size
		}
	}
	if rel.asset == "" {
		return nil, fmt.Errorf("release %s has no build for %s/%s", latest, runtime.GOOS, runtime.GOARCH)
	}
	rel.Method = "replace"
	switch {
	case runtime.GOOS == "windows" && rel.installer != "" && !canReplaceSelf():
		rel.Method, rel.Size = "installer", rel.instSize
	case runtime.GOOS == "linux" && rel.installer != "" && !canReplaceSelf():
		// Installed from a .deb/.rpm: hand the new package to the system installer.
		rel.Method, rel.Size = "package", rel.instSize
	}
	return rel, nil
}

// Newer reports whether version a is newer than b (semver, "v" optional).
// Non-release builds ("dev") are never considered newer.
func Newer(a, b string) bool {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka {
		return false
	}
	if !okb {
		return true
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-") // ignore pre-release suffixes
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Progress receives bytes downloaded so far and the total (0 if unknown).
type Progress func(done, total int64)

// Apply downloads, verifies and installs rel. On success the caller should
// call Restart. The returned bool reports whether an installer was launched
// instead (the app should then quit without relaunching).
func Apply(ctx context.Context, rel *Release, p Progress) (mode string, err error) {
	exe, err := target()
	if err != nil {
		return "", err
	}
	sums, err := checksums(ctx, rel.checksums)
	if err != nil {
		return "", err
	}
	if rel.Method == "package" {
		return "package", openPackage(ctx, rel, sums, p)
	}

	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".quarry-update-*")
	if errors.Is(err, os.ErrPermission) && runtime.GOOS == "windows" && rel.installer != "" {
		return "installer", runInstaller(ctx, rel, sums, p)
	}
	if err != nil {
		return "", fmt.Errorf("can't write to %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := download(ctx, rel.asset, tmp, sums[rel.assetName], p); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}

	// Swap. Windows can't overwrite a running exe but can rename it.
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, exe); err != nil {
		_ = os.Rename(old, exe) // roll back
		return "", err
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(old)
	}
	resign(exe)
	return "replace", nil
}

// Cleanup removes the previous binary left behind by a Windows update.
func Cleanup() {
	if exe, err := os.Executable(); err == nil {
		_ = os.Remove(exe + ".old")
	}
}

// Restart launches the (new) executable and returns; the caller then quits.
func Restart() error {
	exe, err := target()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe)
	cmd.Stdout, cmd.Stderr = nil, nil
	return cmd.Start()
}

func runInstaller(ctx context.Context, rel *Release, sums map[string]string, p Progress) error {
	f, err := os.CreateTemp("", "quarry-setup-*.exe")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := download(ctx, rel.installer, f, sums[rel.instName], p); err != nil {
		f.Close()
		os.Remove(name)
		return err
	}
	f.Close()
	// Silent install; the installer waits for Quarry to exit, then reopens it.
	return launch(name, "/S")
}

func checksums(ctx context.Context, url string) (map[string]string, error) {
	if url == "" {
		return nil, errors.New("release is missing checksums.txt; refusing to install an unverified update")
	}
	resp, err := get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("checksums.txt: %s", resp.Status)
	}
	out := map[string]string{}
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 {
			out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return out, sc.Err()
}

func download(ctx context.Context, url string, w io.Writer, wantSum string, p Progress) error {
	if wantSum == "" {
		return errors.New("no checksum published for this file; refusing to install it")
	}
	dl := &http.Client{} // no overall timeout for large downloads; ctx cancels
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Quarry-Updater")
	resp, err := dl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	h := sha256.New()
	pr := &progressReader{r: resp.Body, total: resp.ContentLength, p: p}
	if _, err := io.Copy(io.MultiWriter(w, h), pr); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != wantSum {
		return fmt.Errorf("checksum mismatch (got %s…, want %s…) — update discarded", got[:12], wantSum[:min(12, len(wantSum))])
	}
	return nil
}

type progressReader struct {
	r     io.Reader
	n     int64
	total int64
	p     Progress
	last  time.Time
}

func (r *progressReader) Read(b []byte) (int, error) {
	n, err := r.r.Read(b)
	r.n += int64(n)
	if r.p != nil && (time.Since(r.last) > 100*time.Millisecond || err == io.EOF) {
		r.last = time.Now()
		r.p(r.n, r.total)
	}
	return n, err
}

// resign re-applies an ad-hoc signature to the macOS app bundle after its
// executable was replaced, so the bundle seal stays valid. Best effort.
func resign(exe string) {
	if runtime.GOOS != "darwin" {
		return
	}
	i := strings.Index(exe, ".app/")
	if i < 0 {
		return
	}
	_ = exec.Command("/usr/bin/codesign", "--force", "--deep", "--sign", "-", exe[:i+4]).Run()
}

// canReplaceSelf reports whether the running executable's folder is writable.
func canReplaceSelf() bool {
	exe, err := target()
	if err != nil {
		return false
	}
	f, err := os.CreateTemp(filepath.Dir(exe), ".quarry-write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// openPackage downloads the release's .deb/.rpm and opens it with the
// desktop's software installer, which asks for permission and upgrades the
// system package. Quarry keeps running; the user restarts it afterwards.
func openPackage(ctx context.Context, rel *Release, sums map[string]string, p Progress) error {
	dir, err := os.MkdirTemp("", "quarry-update-")
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "quarry-"+rel.Version+filepath.Ext(rel.instName))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := download(ctx, rel.installer, f, sums[rel.instName], p); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := exec.Command("xdg-open", path).Start(); err != nil {
		return fmt.Errorf("downloaded %s but couldn't open it (%w) — open it from your file manager", path, err)
	}
	return nil
}
