// Background updates: on launch (at most once daily), the app checks GitHub
// releases for updates. When a newer version is available, a notification is
// shown, and Settings -> Update allows installing it by pressing A: downloads
// the package, verifies SHA256, swaps files atomically, and restarts.
package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/juanvillacortac/audiofilus/internal/proc"
	"github.com/juanvillacortac/audiofilus/internal/update"
	"github.com/juanvillacortac/audiofilus/internal/version"
)

// InstallPackage passes downloaded APK to system package manager on Android.
var (
	InstallPackage func(path string)
	UpdateDir      string
)

// updateState holds state for in-app updates.
type updateState struct {
	exe        string
	kind       update.Kind
	started    bool
	checking   bool
	latest     *update.Release
	err        string
	done       chan updateResult
	installing bool
	dlDone     atomic.Int64
	dlSize     atomic.Int64
}

// updateResult is returned by the update check / download goroutine.
type updateResult struct {
	rel       *update.Release
	installed bool
	err       error
}

// updateFile tracks last check time and notified release tag (data/update.json).
type updateFile struct {
	Checked  time.Time `json:"checked"`
	Notified string    `json:"notified,omitempty"`
}

func (a *App) updatePath() string { return filepath.Join(a.paths.Data, "update.json") }

func (a *App) loadUpdateFile() updateFile {
	var f updateFile
	if b, err := os.ReadFile(a.updatePath()); err == nil {
		_ = json.Unmarshal(b, &f)
	}
	return f
}

func (a *App) saveUpdateFile(f updateFile) {
	if b, err := json.Marshal(f); err == nil {
		_ = os.MkdirAll(a.paths.Data, 0o755)
		_ = os.WriteFile(a.updatePath(), b, 0o644)
	}
}

// canUpdate returns whether self-updating is supported for this install.
func (a *App) canUpdate() bool {
	u := &a.upd
	if !u.started {
		u.started = true
		u.done = make(chan updateResult, 1)
		u.exe, _ = proc.Executable()
		u.kind = update.Detect(u.exe)
		update.Cleanup(u.exe)
	}
	if a.offline || version.Version == "dev" {
		return false
	}
	if a.mobile() {
		return InstallPackage != nil && UpdateDir != ""
	}
	return u.kind != update.None
}

// apkPath returns destination path for Android APK downloads.
func apkPath() string { return filepath.Join(UpdateDir, update.APK) }

// checkUpdate queries GitHub releases for the latest version in background.
func (a *App) checkUpdate() {
	u := &a.upd
	if u.checking || u.installing {
		return
	}
	u.checking, u.err = true, ""
	go func(done chan updateResult) {
		rel, err := update.Latest(context.Background())
		done <- updateResult{rel: rel, err: err}
		ebiten.ScheduleFrame()
	}(u.done)
}

// pollUpdate handles periodic update checks and goroutine completions.
func (a *App) pollUpdate() {
	if a.splashing || !a.canUpdate() {
		return
	}
	u := &a.upd
	if u.latest == nil && !u.checking && u.err == "" {
		if f := a.loadUpdateFile(); time.Since(f.Checked) > 24*time.Hour {
			a.checkUpdate()
		}
	}
	select {
	case r := <-u.done:
		u.checking = false
		if r.installed {
			a.finishUpdate(r.err)
			return
		}
		if r.err != nil {
			u.err = r.err.Error()
			log.Printf("update check: %v", r.err)
			return
		}
		u.latest = r.rel
		f := a.loadUpdateFile()
		f.Checked = time.Now()
		if update.Newer(version.Version, r.rel.Tag) && f.Notified != r.rel.Tag {
			f.Notified = r.rel.Tag
			a.flash(T("upd_available", r.rel.Tag))
		}
		a.saveUpdateFile(f)
	default:
	}
}

// updateAvailable returns true if a newer version has been discovered.
func (a *App) updateAvailable() bool {
	return a.upd.latest != nil && update.Newer(version.Version, a.upd.latest.Tag)
}

// pressUpdate initiates update check or triggers installation.
func (a *App) pressUpdate() {
	u := &a.upd
	switch {
	case !a.canUpdate() || u.checking || u.installing:
	case a.updateAvailable():
		u.installing = true
		u.dlDone.Store(0)
		u.dlSize.Store(-1)
		rel, exe := u.latest, u.exe
		mobile := a.mobile()
		go func(done chan updateResult) {
			progress := func(d, t int64) {
				u.dlDone.Store(d)
				u.dlSize.Store(t)
				ebiten.ScheduleFrame()
			}
			var err error
			if mobile {
				err = update.Fetch(context.Background(), rel, update.APK, apkPath(), progress)
			} else {
				err = update.Install(context.Background(), rel, exe, progress)
			}
			done <- updateResult{installed: true, err: err}
			ebiten.ScheduleFrame()
		}(u.done)
	default:
		a.checkUpdate()
	}
}

// finishUpdate handles completion of update download and installation.
func (a *App) finishUpdate(err error) {
	u := &a.upd
	u.installing = false
	if err != nil {
		log.Printf("update install: %v", err)
		u.err = err.Error()
		a.flash(T("upd_failed", err))
		return
	}
	if a.mobile() {
		log.Printf("update: %s downloaded; handing it to the installer", u.latest.Tag)
		a.flash(T("upd_confirm", u.latest.Tag))
		InstallPackage(apkPath())
		return
	}
	log.Printf("updated to %s; restarting", u.latest.Tag)
	a.send("restart")
	time.Sleep(300 * time.Millisecond) // Allow service to process restart before UI exits
	if err := proc.Reexec(); err != nil {
		u.err = err.Error()
		a.flash(T("upd_restart", u.latest.Tag))
	}
}

// updateValue returns the label string displayed on the settings tile.
func (a *App) updateValue() string {
	u := &a.upd
	switch {
	case !a.canUpdate():
		return version.Version
	case u.installing:
		if t := u.dlSize.Load(); t > 0 {
			if d := u.dlDone.Load(); d < t {
				return T("upd_downloading", int(d*100/t))
			}
			return T("upd_installing")
		}
		return T("upd_downloading", 0)
	case u.checking:
		return T("upd_checking")
	case a.updateAvailable():
		return T("upd_install", u.latest.Tag)
	case u.err != "":
		return T("upd_error")
	case u.latest != nil:
		return T("upd_current", version.Version)
	}
	return fmt.Sprintf("%s · %s", version.Version, T("upd_check"))
}
