package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/juanvillacortac/audiofilus/internal/control"
	"github.com/juanvillacortac/audiofilus/internal/remote"
	"github.com/juanvillacortac/audiofilus/internal/ytm"
)

// isSpotifyPath comprueba si una ruta corresponde a una pista de Spotify.
func isSpotifyPath(path string) bool {
	if path == "" {
		return false
	}
	if strings.HasPrefix(path, "spotify://") || strings.HasPrefix(path, "spotify:") {
		return true
	}
	slash := filepath.ToSlash(path)
	return strings.Contains(slash, "/cache/spotify/")
}

// spotifyDurSec normalizes duration (nanoseconds, milliseconds, or seconds) to seconds.
func spotifyDurSec(d int) int {
	if d <= 0 {
		return 0
	}
	if d > 1e8 {
		return d / 1e9
	}
	if d > 1000 {
		return d / 1000
	}
	return d
}

// isSmartTrack checks whether a song path has been injected by Spotify Smart Shuffle.
func isSmartTrack(path string) bool {
	return strings.Contains(path, "smart=1")
}

// extractSpotifyID extracts raw track ID from Spotify URIs or file paths.
func extractSpotifyID(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "spotify://track/") {
		path = strings.TrimPrefix(path, "spotify://track/")
	} else if strings.HasPrefix(path, "spotify:track:") {
		path = strings.TrimPrefix(path, "spotify:track:")
	} else if strings.HasPrefix(path, "spotify://") {
		path = strings.TrimPrefix(path, "spotify://")
	} else if strings.HasPrefix(path, "spotify:") {
		path = strings.TrimPrefix(path, "spotify:")
	} else {
		base := filepath.Base(path)
		ext := filepath.Ext(base)
		path = strings.TrimSuffix(base, ext)
	}
	if idx := strings.IndexAny(path, "?#"); idx != -1 {
		path = path[:idx]
	}
	return path
}

// spotifyMusicDirs returns configured local music directories or default SD-card paths.
func (a *App) spotifyMusicDirs() []string {
	if len(a.cfg.MusicDirs) > 0 {
		return append([]string(nil), a.cfg.MusicDirs...)
	}
	gen := a.spot.dlGen.Load()
	now := time.Now()
	a.spot.dlMu.Lock()
	if a.spot.dirsCached && a.spot.dirsGen == gen && a.spot.dirsBase == a.paths.Base && now.Before(a.spot.dirsExpAt) {
		out := append([]string(nil), a.spot.dirsCache...)
		a.spot.dlMu.Unlock()
		return out
	}
	a.spot.dlMu.Unlock()

	var dirs []string
	candidates := []string{
		"/mnt/mmc/Musics",
		"/mnt/SDCARD/Music",
		"/mnt/sdcard/Music",
		"/mnt/mmc/MUOS/music",
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && fi.IsDir() {
			dirs = append(dirs, c)
			break
		}
	}
	if len(dirs) == 0 && a.paths.Base != "" {
		dirs = []string{filepath.Join(a.paths.Base, "Music")}
	}

	a.spot.dlMu.Lock()
	a.spot.dirsCached = true
	a.spot.dirsGen = gen
	a.spot.dirsBase = a.paths.Base
	a.spot.dirsCache = append([]string(nil), dirs...)
	a.spot.dirsExpAt = time.Now().Add(30 * time.Second)
	a.spot.dlMu.Unlock()
	return dirs
}

func (a *App) ensureSpotifyMusicDir() string {
	if len(a.cfg.MusicDirs) > 0 && a.cfg.MusicDirs[0] != "" {
		return a.cfg.MusicDirs[0]
	}
	dirs := a.spotifyMusicDirs()
	musicDir := ""
	if len(dirs) > 0 {
		musicDir = dirs[0]
	}
	if musicDir == "" {
		musicDir = "Music"
	}
	_ = os.MkdirAll(musicDir, 0o755)
	a.cfg.MusicDirs = []string{musicDir}
	a.saveConfig()
	return musicDir
}

func pathInsideDir(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (a *App) matchLocalLibEntry(tr spotifyTrack) string {
	if tr.Title == "" || tr.Title == "Spotify Track" {
		return ""
	}
	wantBase, _ := ytm.BaseTitleAndVersion(tr.Title)
	if wantBase == "" {
		return ""
	}

	a.spot.dlMu.Lock()
	lib := a.spot.libSnap
	if lib == nil {
		lib = a.local
	}
	if lib == nil {
		lib = a.lib
	}
	var idx map[string][]spotLibEntry
	if lib != nil && a.spot.libIndexOf == lib {
		idx = a.spot.libIndex
	}
	a.spot.dlMu.Unlock()

	if lib != nil && idx == nil {
		// Build the immutable base-title index outside dlMu so UI threads never block on large libraries.
		built := make(map[string][]spotLibEntry, len(lib.Songs))
		for _, s := range lib.Songs {
			if s.Path == "" || remote.IsRemote(s.Path) || s.Title == "" {
				continue
			}
			base, _ := ytm.BaseTitleAndVersion(s.Title)
			if base == "" {
				continue
			}
			built[base] = append(built[base], spotLibEntry{
				path:        s.Path,
				title:       s.Title,
				artist:      s.Artist,
				albumArtist: lib.meta[s.Path].albumArtist,
			})
		}
		a.spot.dlMu.Lock()
		if a.spot.libIndexOf != lib {
			a.spot.libIndex = built
			a.spot.libIndexOf = lib
		}
		idx = a.spot.libIndex
		a.spot.dlMu.Unlock()
	}

	var cands []spotLibEntry
	if idx != nil {
		cands = idx[wantBase]
	}

	for _, c := range cands {
		if !ytm.ArtistMatches(tr.Artist, c.artist, c.albumArtist) {
			continue
		}
		if !ytm.TitleMatches(c.title, tr.Title, false) {
			continue
		}
		if fi, err := os.Stat(c.path); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return c.path
		}
	}
	return ""
}

// isSpotifyTrackDownloaded checks if a track has already been downloaded and exists on disk.
func (a *App) isSpotifyTrackDownloaded(tr spotifyTrack) (string, bool) {
	p, ok, _ := a.checkSpotifyTrackDownloaded(tr, true)
	return p, ok
}

func (a *App) checkSpotifyTrackDownloaded(tr spotifyTrack, bumpRev bool) (string, bool, bool) {
	if tr.ID == "" {
		return "", false, false
	}

	gen := a.spot.dlGen.Load()
	now := time.Now()
	a.spot.dlMu.Lock()
	if c, ok := a.spot.dlCache[tr.ID]; ok && c.gen == gen && now.Before(c.expAt) {
		a.spot.dlMu.Unlock()
		return c.path, c.ok, false
	}
	a.spot.dlMu.Unlock()

	cacheResult := func(p string, found bool) (string, bool, bool) {
		ttl := 30 * time.Second
		if found {
			ttl = 60 * time.Second
		}
		a.spot.dlMu.Lock()
		if a.spot.dlCache == nil {
			a.spot.dlCache = make(map[string]dlCacheEntry)
		}
		a.spot.dlCache[tr.ID] = dlCacheEntry{
			path:  p,
			ok:    found,
			gen:   gen,
			expAt: time.Now().Add(ttl),
		}
		a.spot.dlMu.Unlock()
		if bumpRev || found {
			a.spot.dlRev.Add(1)
		}
		return p, found, true
	}

	dirs := a.spotifyMusicDirs()

	// 1. Check known IDs from .library.json files
	a.yt.mu.Lock()
	hasID := a.yt.ids != nil && (a.yt.ids["spotify:"+tr.ID] || a.yt.ids[tr.ID])
	idPath := ""
	if a.yt.idPaths != nil {
		if p := a.yt.idPaths["spotify:"+tr.ID]; p != "" {
			idPath = p
		} else if p := a.yt.idPaths[tr.ID]; p != "" {
			idPath = p
		}
	}
	a.yt.mu.Unlock()

	if idPath != "" {
		if fi, err := os.Stat(idPath); err == nil && !fi.IsDir() && fi.Size() > 0 {
			return cacheResult(idPath, true)
		}
	}
	if hasID {
		for _, d := range dirs {
			lib := ytm.LoadLibrary(d)
			for _, key := range []string{"spotify:" + tr.ID, tr.ID} {
				if rel, ok := lib[key]; ok && rel != "" {
					p := filepath.Join(d, rel)
					if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
						return cacheResult(p, true)
					}
				}
			}
		}
	}

	// 2. Check local scanned library using version-aware title and multi-artist matching.
	if p := a.matchLocalLibEntry(tr); p != "" {
		return cacheResult(p, true)
	}

	// 3. Check conventional, flat, alternate-album, and truncated paths on disk.
	if tr.Title != "" && tr.Title != "Spotify Track" {
		if p := ytm.FindLocalAudio(dirs, tr.Artist, tr.Album, tr.Title); p != "" {
			return cacheResult(p, true)
		}
	}

	// 4. Check completed jobs in current session.
	for _, j := range a.ytJobsSnapshot() {
		if (j.track.ID == "spotify:"+tr.ID || j.track.ID == tr.ID) && !j.listen && j.state == jobDone {
			if j.path != "" {
				if fi, err := os.Stat(j.path); err == nil && !fi.IsDir() && fi.Size() > 0 {
					return cacheResult(j.path, true)
				}
			}
		}
	}
	return cacheResult("", false)
}

// cachedSpotifyTrackDownloaded returns cached download status without performing synchronous disk I/O,
// queuing uncached tracks for background verification so UI rendering stays at 60 FPS.
func (a *App) cachedSpotifyTrackDownloaded(tr spotifyTrack) (string, bool) {
	if tr.ID == "" {
		return "", false
	}
	gen := a.spot.dlGen.Load()
	now := time.Now()
	a.spot.dlMu.Lock()
	if c, ok := a.spot.dlCache[tr.ID]; ok && c.gen == gen && now.Before(c.expAt) {
		a.spot.dlMu.Unlock()
		return c.path, c.ok
	}
	a.enqueueSpotifyDlCheckLocked([]spotifyTrack{tr}, gen)
	a.spot.dlMu.Unlock()
	return "", false
}

// playlistDownloadCount returns the memoized downloaded track count for pl without blocking on disk I/O.
func (a *App) playlistDownloadCount(pl spotifyPlaylist) (int, bool) {
	if len(pl.Tracks) == 0 {
		return 0, true
	}
	key := pl.ID
	if key == "" {
		key = "name:" + pl.Name
	}
	total := pl.TrackCount
	if total <= 0 || len(pl.Tracks) > total {
		total = len(pl.Tracks)
	}
	gen := a.spot.dlGen.Load()
	rev := a.spot.dlRev.Load()
	now := time.Now()

	a.spot.dlMu.Lock()
	if a.spot.plDown != nil {
		if e, ok := a.spot.plDown[key]; ok && e.gen == gen && e.rev == rev && e.total == total && e.loaded == len(pl.Tracks) && e.snapID == pl.SnapshotID && now.Before(e.expAt) {
			a.spot.dlMu.Unlock()
			return e.down, e.complete
		}
	}

	down := 0
	complete := true
	earliestExp := now.Add(60 * time.Second)
	var missing []spotifyTrack
	for _, t := range pl.Tracks {
		if t.ID == "" {
			continue
		}
		if c, ok := a.spot.dlCache[t.ID]; ok && c.gen == gen && now.Before(c.expAt) {
			if c.ok {
				down++
			}
			if c.expAt.Before(earliestExp) {
				earliestExp = c.expAt
			}
		} else {
			complete = false
			missing = append(missing, t)
		}
	}
	if len(pl.Tracks) < total {
		complete = false
	}
	if a.spot.plDown == nil {
		a.spot.plDown = make(map[string]plDownEntry)
	}
	a.spot.plDown[key] = plDownEntry{
		down:     down,
		total:    total,
		loaded:   len(pl.Tracks),
		snapID:   pl.SnapshotID,
		complete: complete,
		gen:      gen,
		rev:      rev,
		expAt:    earliestExp,
	}
	if len(missing) > 0 {
		a.enqueueSpotifyDlCheckLocked(missing, gen)
	}
	a.spot.dlMu.Unlock()
	return down, complete
}

func (a *App) enqueueSpotifyDlCheckLocked(tracks []spotifyTrack, gen uint64) {
	if len(tracks) == 0 {
		return
	}
	s := &a.spot
	if s.dlPending == nil {
		s.dlPending = make(map[string]uint64)
	}
	now := time.Now()
	added := false
	for _, tr := range tracks {
		if tr.ID == "" {
			continue
		}
		if s.dlPending[tr.ID] == gen {
			continue
		}
		if c, ok := s.dlCache[tr.ID]; ok && c.gen == gen && now.Before(c.expAt) {
			continue
		}
		s.dlPending[tr.ID] = gen
		s.dlQueue = append(s.dlQueue, dlQueueItem{tr: tr, gen: gen})
		added = true
	}
	if added && !s.dlWorkerOn {
		s.dlWorkerOn = true
		if s.dlIdleCh == nil {
			s.dlIdleCh = make(chan struct{})
		}
		go a.runSpotifyDlWorker()
	}
}

func (a *App) runSpotifyDlWorker() {
	s := &a.spot
	unflushed := 0
	for {
		s.dlMu.Lock()
		if len(s.dlQueue) == 0 {
			if unflushed > 0 {
				s.dlRev.Add(1)
				unflushed = 0
			}
			s.dlWorkerOn = false
			if s.dlIdleCh != nil {
				close(s.dlIdleCh)
				s.dlIdleCh = nil
			}
			s.dlMu.Unlock()
			return
		}
		item := s.dlQueue[0]
		s.dlQueue[0] = dlQueueItem{}
		s.dlQueue = s.dlQueue[1:]
		if len(s.dlQueue) == 0 {
			s.dlQueue = nil
		}
		s.dlMu.Unlock()

		if item.gen == s.dlGen.Load() {
			_, found, updated := a.checkSpotifyTrackDownloaded(item.tr, false)
			if updated && !found {
				unflushed++
				if unflushed >= 16 {
					s.dlRev.Add(1)
					unflushed = 0
				}
			}
			if found {
				unflushed = 0
			}
		}

		s.dlMu.Lock()
		if s.dlPending != nil && s.dlPending[item.tr.ID] == item.gen {
			delete(s.dlPending, item.tr.ID)
		}
		s.dlMu.Unlock()
	}
}

// waitSpotifyDlChecks waits for all queued background download-detection checks to finish (used by tests).
func (a *App) waitSpotifyDlChecks() {
	for {
		a.spot.dlMu.Lock()
		if !a.spot.dlWorkerOn {
			a.spot.dlMu.Unlock()
			return
		}
		ch := a.spot.dlIdleCh
		a.spot.dlMu.Unlock()
		if ch == nil {
			return
		}
		<-ch
	}
}

// isSpotifyTrackInFlight checks whether a download for tr (by ID or artist+title) is queued or in progress.
func (a *App) isSpotifyTrackInFlight(tr spotifyTrack) bool {
	for _, j := range a.ytJobsSnapshot() {
		if j.listen || !j.active() {
			continue
		}
		if j.track.ID == "spotify:"+tr.ID || j.track.ID == tr.ID {
			return true
		}
		if tr.Title != "" && ytm.TitleMatches(j.track.Title, tr.Title, false) && ytm.ArtistMatches(tr.Artist, j.track.Artist(), "") {
			return true
		}
	}
	return false
}

// downloadSpotifyTrack adds a Spotify track to the unified download queue.
func (a *App) downloadSpotifyTrack(tr spotifyTrack) {
	if tr.ID == "" {
		return
	}
	if _, ok := a.isSpotifyTrackDownloaded(tr); ok {
		a.flash(T("spotify_downloaded"))
		return
	}
	if a.isSpotifyTrackInFlight(tr) {
		a.flash(T("ytm_already"))
		return
	}
	musicDir := a.ensureSpotifyMusicDir()
	a.enqueueSpotifyTrack(tr, musicDir)
	a.flash(T("ytm_queued", tr.Title))
}

func (a *App) enqueueSpotifyTrack(tr spotifyTrack, musicDir string) {
	j := &ytJob{
		track: ytm.Track{
			ID:      "spotify:" + tr.ID,
			Title:   tr.Title,
			Artists: []string{tr.Artist},
			Album:   tr.Album,
		},
		remote: true,
		state:  jobWaiting,
	}
	j.after = func(destPath string) {
		a.yt.mu.Lock()
		if a.yt.ids == nil {
			a.yt.ids = map[string]bool{}
		}
		if a.yt.idPaths == nil {
			a.yt.idPaths = map[string]string{}
		}
		a.yt.ids["spotify:"+tr.ID] = true
		a.yt.ids[tr.ID] = true
		if destPath != "" {
			a.yt.idPaths["spotify:"+tr.ID] = destPath
			a.yt.idPaths[tr.ID] = destPath
		}
		a.yt.mu.Unlock()
		a.spot.dlGen.Add(1)
	}
	j.run = func(ctx context.Context, progress func(ytm.Progress)) (string, error) {
		progress(ytm.Progress{Stage: "resolve"})
		if a.client != nil {
			a.send("spotify", "download", tr.ID)
		}
		progress(ytm.Progress{Stage: "download", Done: 10, Total: 100})
		ticker := time.NewTicker(800 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(3 * time.Minute)
		curProg := int64(20)
		for {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-timeout:
				return "", fmt.Errorf("timeout")
			case <-ticker.C:
				if a.client != nil {
					var res struct {
						Cached bool   `json:"cached"`
						Path   string `json:"path"`
					}
					if err := a.client.CallJSON(&res, "spotify", "cache_path", tr.ID); err == nil && res.Cached && res.Path != "" {
						dirs := a.spotifyMusicDirs()
						for _, d := range dirs {
							if pathInsideDir(res.Path, d) {
								if rel, err := filepath.Rel(d, res.Path); err == nil {
									lib := ytm.LoadLibrary(d)
									lib["spotify:"+tr.ID] = rel
									lib[tr.ID] = rel
									_ = ytm.SaveLibrary(d, lib)
								}
								progress(ytm.Progress{Stage: "done", Done: 100, Total: 100})
								return res.Path, nil
							}
						}

						progress(ytm.Progress{Stage: "convert", Done: 95, Total: 100})
						t := ytm.Track{
							ID:      tr.ID,
							Title:   tr.Title,
							Artists: []string{tr.Artist},
							Album:   tr.Album,
						}
						// Check whether cover art is saved alongside the file.
						var coverBytes []byte
						covPath := strings.TrimSuffix(res.Path, filepath.Ext(res.Path)) + ".jpg"
						if b, err := os.ReadFile(covPath); err == nil {
							coverBytes = b
						} else if tr.CoverURL != "" {
							if req, err := http.NewRequestWithContext(ctx, "GET", tr.CoverURL, nil); err == nil {
								if resp, err := http.DefaultClient.Do(req); err == nil && resp.StatusCode == 200 {
									if b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)); err == nil {
										coverBytes = b
									}
									resp.Body.Close()
								}
							}
						}
						targetDir := musicDir
						if len(dirs) > 0 && dirs[0] != "" {
							targetDir = dirs[0]
						}
						destPath, err := ytm.SaveM4A(t, res.Path, targetDir, coverBytes, progress)
						if err != nil {
							return "", err
						}
						if rel, err := filepath.Rel(targetDir, destPath); err == nil {
							lib := ytm.LoadLibrary(targetDir)
							lib["spotify:"+tr.ID] = rel
							lib[tr.ID] = rel
							_ = ytm.SaveLibrary(targetDir, lib)
						}
						progress(ytm.Progress{Stage: "done", Done: 100, Total: 100})
						return destPath, nil
					}
				}
				if curProg < 90 {
					curProg += 10
				}
				progress(ytm.Progress{Stage: "download", Done: curProg, Total: 100})
			}
		}
	}
	a.addJob(j)
	a.spot.dlGen.Add(1)
	if a.yt.queue != nil {
		select {
		case a.yt.queue <- j:
		default:
			go func() { a.yt.queue <- j }()
		}
	}
}

// downloadSpotifyTracks queues multiple Spotify tracks for download after filtering already downloaded or duplicate items.
func (a *App) downloadSpotifyTracks(name string, tracks []spotifyTrack) {
	if len(tracks) == 0 {
		return
	}
	musicDir := a.ensureSpotifyMusicDir()
	a.downloadSpotifyTracksTo(name, tracks, musicDir)
}

func (a *App) downloadSpotifyTracksTo(name string, tracks []spotifyTrack, musicDir string) {
	if len(tracks) == 0 {
		return
	}
	seenIDs := make(map[string]bool, len(tracks))
	seenKeys := make(map[string]bool, len(tracks))
	var missing []spotifyTrack
	for _, tr := range tracks {
		if tr.ID == "" || seenIDs[tr.ID] {
			continue
		}
		seenIDs[tr.ID] = true
		if tr.Title != "" {
			base, ver := ytm.BaseTitleAndVersion(tr.Title)
			k := ytm.CleanWords(ytm.FirstArtist(tr.Artist)) + "|" + base + "|" + ver
			if base != "" && seenKeys[k] {
				continue
			}
			seenKeys[k] = true
		}
		if _, downloaded := a.isSpotifyTrackDownloaded(tr); !downloaded && !a.isSpotifyTrackInFlight(tr) {
			missing = append(missing, tr)
		}
	}
	if len(missing) == 0 {
		a.flash(T("spotify_all_downloaded", name))
		return
	}
	for _, tr := range missing {
		a.enqueueSpotifyTrack(tr, musicDir)
	}
	a.flash(T("spotify_downloading_all", name, len(missing)))
}

// createOrUpdateSpotifyLibPlaylist creates or updates a playlist in the user's Library corresponding to a Spotify playlist.
func (a *App) createOrUpdateSpotifyLibPlaylist(pl spotifyPlaylist) *Playlist {
	if a.lib == nil {
		a.lib = &Library{song: map[string]Song{}}
	}
	dir := a.playlistsDir()
	name := strings.TrimSpace(pl.Name)
	if name == "" {
		name = "Spotify"
	}
	var existing *Playlist
	for _, p := range a.lib.Playlists {
		if strings.EqualFold(p.Name, name) {
			existing = p
			break
		}
	}
	var targetPath string
	if existing != nil {
		targetPath = existing.Path
	} else {
		targetPath = playlistFile(dir, name, ".m3u8")
	}

	var songs []Song
	for _, tr := range pl.Tracks {
		id := extractSpotifyID(tr.ID)
		if id == "" {
			continue
		}
		path := "spotify://track/" + id
		if localPath, ok := a.isSpotifyTrackDownloaded(tr); ok && localPath != "" {
			path = localPath
		}
		title := tr.Title
		if title == "" {
			title = id
		}
		songs = append(songs, Song{
			Path:   path,
			Title:  title,
			Artist: tr.Artist,
			Album:  tr.Album,
		})
	}

	if existing != nil {
		existing.Songs = songs
		existing.icon = "spotify"
		if err := a.savePlaylist(existing); err != nil {
			a.flash(T("pl_error", err))
			return existing
		}
		a.refreshPlaylists()
		return existing
	}

	newPl := &Playlist{
		Name:  name,
		Path:  targetPath,
		icon:  "spotify",
		Songs: songs,
	}
	if err := a.savePlaylist(newPl); err != nil {
		a.flash(T("pl_error", err))
		return nil
	}
	a.lib.Playlists = append(a.lib.Playlists, newPl)
	slices.SortFunc(a.lib.Playlists, func(x, y *Playlist) int { return strings.Compare(fold(x.Name), fold(y.Name)) })
	a.refreshPlaylists()
	return newPl
}

// downloadSpotifyPlaylist queues all tracks of a playlist for download after filtering already downloaded items.
func (a *App) downloadSpotifyPlaylist(pl spotifyPlaylist) {
	musicDir := a.ensureSpotifyMusicDir()
	if len(pl.Tracks) > 0 {
		a.createOrUpdateSpotifyLibPlaylist(pl)
	}
	go func() {
		tracks := pl.Tracks
		subCmd := spotifySubcommandForPlaylist(pl)
		if (len(tracks) == 0 || (pl.TrackCount > 0 && len(tracks) < pl.TrackCount)) && a.paths.Socket != "" {
			if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{subCmd, pl.ID}}); err == nil {
				var fetched []spotifyTrack
				if err := json.Unmarshal(resp.Data, &fetched); err == nil && len(fetched) > 0 {
					tracks = fetched
					pl.Tracks = fetched
					a.postUI(func() {
						a.createOrUpdateSpotifyLibPlaylist(pl)
					})
				}
			}
		}
		if len(tracks) == 0 {
			return
		}
		if len(pl.Tracks) == 0 {
			pl.Tracks = tracks
			a.postUI(func() {
				a.createOrUpdateSpotifyLibPlaylist(pl)
			})
		}
		a.downloadSpotifyTracksTo(pl.Name, tracks, musicDir)
	}()
}

// Spotify tab constants.
const (
	spSearch = iota
	spRecent
	spLiked
	spPlaylists
	spAlbums
	spCount
)

var spotifyTabKeys = [spCount]string{
	"spotify_search",
	"spotify_recent",
	"spotify_liked",
	"spotify_playlists",
	"spotify_albums",
}

// spotifyTrack represents a Spotify track in the UI.
type spotifyTrack struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Duration int    `json:"duration"`
	URI      string `json:"uri"`
	ISRC     string `json:"isrc"`
	CoverURL string `json:"cover_url,omitempty"`
}

// spotifyPlaylist represents a Spotify playlist in the UI.
type spotifyPlaylist struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	SnapshotID string         `json:"snapshot_id"`
	TrackCount int            `json:"track_count,omitempty"`
	IsAlbum    bool           `json:"is_album,omitempty"`
	Tracks     []spotifyTrack `json:"tracks"`
}

// spotifySubcommandForPlaylist returns the daemon subcommand to fetch tracks for a playlist or album.
func spotifySubcommandForPlaylist(pl spotifyPlaylist) string {
	if pl.IsAlbum {
		return "album_tracks"
	}
	return "playlist_tracks"
}

// dlCacheEntry caches local download detection results per track for smooth 60 FPS UI rendering.
type dlCacheEntry struct {
	path  string
	ok    bool
	gen   uint64
	expAt time.Time
}

// plDownEntry caches playlist-level download totals so drawing playlist rows is O(1).
type plDownEntry struct {
	down     int
	total    int
	loaded   int
	snapID   string
	complete bool
	gen      uint64
	rev      uint64
	expAt    time.Time
}

type dlQueueItem struct {
	tr  spotifyTrack
	gen uint64
}

// spotLibEntry stores minimal local library fields indexed by normalized base title.
type spotLibEntry struct {
	path        string
	title       string
	artist      string
	albumArtist string
}

// spotifyState stores UI state for the Spotify screen.
type spotifyState struct {
	tab            int
	tabFocus       bool
	sel            [spCount]int
	scr            [spCount]scroller
	recent         []spotifyTrack
	liked          []spotifyTrack
	playlists      []spotifyPlaylist
	albums         []spotifyPlaylist
	searchRes      []spotifyTrack
	openPl         *spotifyPlaylist
	openPlSel      int
	openPlScr      scroller
	query          string
	loggedIn       bool
	product        string
	loginURL       string
	loading        bool
	loadingMsg     string
	lastCheck      time.Time
	checkingStatus atomic.Bool
	syncing        atomic.Bool
	likedTotal     int
	likedLoaded    bool
	pageFetching   atomic.Bool

	metaMu      sync.Mutex
	metaCache   map[string]spotifyTrack
	metaPending map[string]bool

	dlMu       sync.Mutex
	dlGen      atomic.Uint64
	dlRev      atomic.Uint64
	dlCache    map[string]dlCacheEntry
	plDown     map[string]plDownEntry
	dlPending  map[string]uint64
	dlQueue    []dlQueueItem
	dlWorkerOn bool
	dlIdleCh   chan struct{}
	dirsCached bool
	dirsGen    uint64
	dirsBase   string
	dirsCache  []string
	dirsExpAt  time.Time
	libSnap    *Library
	libIndexOf *Library
	libIndex   map[string][]spotLibEntry
}

func (s *spotifyState) invalidateLib(lib *Library) {
	s.dlMu.Lock()
	s.libSnap = lib
	s.libIndexOf = nil
	s.libIndex = nil
	s.dlMu.Unlock()
	s.dlGen.Add(1)
}

// initSpotify initializes Spotify UI state.
func (a *App) initSpotify() {
	a.spot.tab = spSearch
	a.spot.dlGen.Add(1)
	go a.checkSpotifyStatus()
	go a.loadCachedSpotifyCatalog()
}

// postUI runs fn on the main Ebitengine update goroutine via acctCh when available.
func (a *App) postUI(fn func()) {
	if fn == nil {
		return
	}
	if a.acctCh == nil {
		fn()
		return
	}
	select {
	case a.acctCh <- fn:
	default:
	}
}

// playSpotifyTracks queues a full Spotify track list at start (skipping entries without an ID)
// so next/previous track skipping navigates across the list.
func (a *App) playSpotifyTracks(tracks []spotifyTrack, start int) {
	if len(tracks) == 0 {
		return
	}
	args := make([]string, 1, len(tracks)+1)
	filteredStart := -1
	for i, tr := range tracks {
		if tr.ID == "" {
			continue
		}
		if i >= start && filteredStart < 0 {
			filteredStart = len(args) - 1
		}
		args = append(args, "spotify://track/"+tr.ID)
	}
	if len(args) <= 1 {
		return
	}
	if filteredStart < 0 {
		filteredStart = len(args) - 2
	}
	args[0] = strconv.Itoa(filteredStart)
	a.send("load", args...)
	a.screen = NowPlaying
}

// playSpotifyCollection starts immediate playback of the chosen track and already-loaded items
// and instructs the daemon to hydrate remaining track IDs in the background.
func (a *App) playSpotifyCollection(collType, id string, tracks []spotifyTrack, start int) {
	if len(tracks) == 0 {
		return
	}
	args := make([]string, 0, len(tracks)+4)
	args = append(args, "play_collection", collType, id)
	filteredStart := -1
	for i, tr := range tracks {
		if tr.ID == "" {
			continue
		}
		if i >= start && filteredStart < 0 {
			filteredStart = len(args) - 3
		}
		args = append(args, tr.ID)
	}
	if len(args) <= 3 {
		return
	}
	if filteredStart < 0 {
		filteredStart = len(args) - 4
	}
	fullArgs := make([]string, 0, len(args)+1)
	fullArgs = append(fullArgs, args[:3]...)
	fullArgs = append(fullArgs, strconv.Itoa(filteredStart))
	fullArgs = append(fullArgs, args[3:]...)

	a.send("spotify", fullArgs...)
	a.screen = NowPlaying
}

// shouldFetchNextSpotifyPage returns true if the user scrolled near the bottom of a partially loaded collection.
func (a *App) shouldFetchNextSpotifyPage(pl *spotifyPlaylist, sel int) bool {
	if pl == nil || len(pl.Tracks) == 0 {
		return false
	}
	if pl.TrackCount > 0 && len(pl.Tracks) >= pl.TrackCount {
		return false
	}
	return sel >= len(pl.Tracks)-5
}

// fetchNextSpotifyPage requests the next page of tracks for an open playlist or album.
func (a *App) fetchNextSpotifyPage() {
	s := &a.spot
	if a.offline || a.paths.Socket == "" || s.openPl == nil {
		return
	}
	if !s.pageFetching.CompareAndSwap(false, true) {
		return
	}
	pl := s.openPl
	offset := len(pl.Tracks)
	subcmd := "playlist_page"
	if pl.IsAlbum {
		subcmd = "album_page"
	}
	go func(plID string, isAlbum bool, offset int) {
		defer s.pageFetching.Store(false)
		resp, err := control.Call(a.paths.Socket, control.Request{
			Cmd:  "spotify",
			Args: []string{subcmd, plID, strconv.Itoa(offset), "50"},
		})
		if err != nil {
			return
		}
		var res struct {
			Tracks []spotifyTrack `json:"tracks"`
			Total  int            `json:"total"`
		}
		if err := json.Unmarshal(resp.Data, &res); err == nil && len(res.Tracks) > 0 {
			a.postUI(func() {
				if s.openPl == nil || s.openPl.ID != plID {
					return
				}
				s.openPl.TrackCount = res.Total
				existing := make(map[string]bool, len(s.openPl.Tracks))
				for _, tr := range s.openPl.Tracks {
					existing[tr.ID] = true
				}
				for _, tr := range res.Tracks {
					if tr.ID != "" && !existing[tr.ID] {
						existing[tr.ID] = true
						s.openPl.Tracks = append(s.openPl.Tracks, tr)
					}
				}
				targetList := &s.playlists
				if isAlbum {
					targetList = &s.albums
				}
				for i := range *targetList {
					if (*targetList)[i].ID == plID {
						(*targetList)[i].Tracks = s.openPl.Tracks
						(*targetList)[i].TrackCount = res.Total
						break
					}
				}
			})
		}
	}(pl.ID, pl.IsAlbum, offset)
}

// fetchNextSpotifyLikedPage requests the next page of liked tracks.
func (a *App) fetchNextSpotifyLikedPage() {
	s := &a.spot
	if a.offline || a.paths.Socket == "" {
		return
	}
	if !s.pageFetching.CompareAndSwap(false, true) {
		return
	}
	offset := len(s.liked)
	go func(offset int) {
		defer s.pageFetching.Store(false)
		resp, err := control.Call(a.paths.Socket, control.Request{
			Cmd:  "spotify",
			Args: []string{"liked_page", strconv.Itoa(offset), "50", "false"},
		})
		if err != nil {
			return
		}
		var res struct {
			Tracks []spotifyTrack `json:"tracks"`
			Total  int            `json:"total"`
		}
		if err := json.Unmarshal(resp.Data, &res); err == nil && len(res.Tracks) > 0 {
			a.postUI(func() {
				s.likedTotal = res.Total
				existing := make(map[string]bool, len(s.liked))
				for _, tr := range s.liked {
					existing[extractSpotifyID(tr.ID)] = true
				}
				for _, tr := range res.Tracks {
					id := extractSpotifyID(tr.ID)
					if id != "" && !existing[id] {
						existing[id] = true
						s.liked = append(s.liked, tr)
					}
				}
			})
		}
	}(offset)
}

// spotifyTrackMetadata returns cached metadata for a Spotify track ID if known.
// If missing or unindexed, it requests metadata resolution from the daemon in the background.
func (a *App) spotifyTrackMetadata(id string) (spotifyTrack, bool) {
	if id == "" {
		return spotifyTrack{}, false
	}
	a.spot.metaMu.Lock()
	if a.spot.metaCache != nil {
		if tr, ok := a.spot.metaCache[id]; ok && tr.Title != "" && tr.Title != "Spotify Track" {
			a.spot.metaMu.Unlock()
			return tr, true
		}
	}
	if a.spot.metaPending == nil {
		a.spot.metaPending = make(map[string]bool)
	}
	if a.spot.metaPending[id] {
		a.spot.metaMu.Unlock()
		return spotifyTrack{}, false
	}
	a.spot.metaPending[id] = true
	a.spot.metaMu.Unlock()

	go a.fetchMissingSpotifyMetadata([]string{id})
	return spotifyTrack{}, false
}

// fetchMissingSpotifyMetadata queries the daemon for metadata of unindexed Spotify track IDs in batches.
func (a *App) fetchMissingSpotifyMetadata(ids []string) {
	if a.offline || a.paths.Socket == "" || len(ids) == 0 {
		return
	}
	args := append([]string{"tracks"}, ids...)
	resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: args})
	if err != nil {
		a.spot.metaMu.Lock()
		for _, id := range ids {
			delete(a.spot.metaPending, id)
		}
		a.spot.metaMu.Unlock()
		return
	}
	var res map[string]spotifyTrack
	if err := json.Unmarshal(resp.Data, &res); err == nil && len(res) > 0 {
		a.spot.metaMu.Lock()
		if a.spot.metaCache == nil {
			a.spot.metaCache = make(map[string]spotifyTrack)
		}
		for id, tr := range res {
			a.spot.metaCache[id] = tr
			delete(a.spot.metaPending, id)
		}
		a.spot.metaMu.Unlock()
	} else {
		a.spot.metaMu.Lock()
		for _, id := range ids {
			delete(a.spot.metaPending, id)
		}
		a.spot.metaMu.Unlock()
	}
}

// addRecentSpotify adds a track to the recent list avoiding duplicates.
func (a *App) addRecentSpotify(tr spotifyTrack) {
	if tr.ID == "" {
		return
	}
	var updated []spotifyTrack
	updated = append(updated, tr)
	for _, t := range a.spot.recent {
		if t.ID != tr.ID && len(updated) < 50 {
			updated = append(updated, t)
		}
	}
	a.spot.recent = updated
}

// loadCachedSpotifyCatalog loads playlists, liked tracks, and albums from daemon cache.
func (a *App) loadCachedSpotifyCatalog() {
	if a.paths.Socket == "" {
		return
	}
	if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_playlists"}}); err == nil {
		var pls []spotifyPlaylist
		if err := json.Unmarshal(resp.Data, &pls); err == nil && len(pls) > 0 {
			a.postUI(func() { a.spot.playlists = pls })
		}
	}
	if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_liked"}}); err == nil {
		var liked []spotifyTrack
		if err := json.Unmarshal(resp.Data, &liked); err == nil && len(liked) > 0 {
			a.postUI(func() { a.spot.liked = liked })
		}
	}
	if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked_page", "0", "50", "false"}}); err == nil {
		var res struct {
			Tracks []spotifyTrack `json:"tracks"`
			Total  int            `json:"total"`
		}
		if err := json.Unmarshal(resp.Data, &res); err == nil {
			a.postUI(func() {
				a.spot.likedTotal = res.Total
				if len(a.spot.liked) == 0 && len(res.Tracks) > 0 {
					a.spot.liked = res.Tracks
				}
			})
		}
	}
	if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_albums"}}); err == nil {
		var albums []spotifyPlaylist
		if err := json.Unmarshal(resp.Data, &albums); err == nil && len(albums) > 0 {
			for i := range albums {
				albums[i].IsAlbum = true
			}
			a.postUI(func() { a.spot.albums = albums })
		}
	}
}

// checkSpotifyStatus queries the daemon for Spotify login status.
func (a *App) checkSpotifyStatus() {
	if a.offline || a.paths.Socket == "" || !a.spot.checkingStatus.CompareAndSwap(false, true) {
		return
	}
	defer a.spot.checkingStatus.Store(false)

	resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"status"}})
	if err != nil {
		return
	}
	var st struct {
		LoggedIn bool   `json:"logged_in"`
		Product  string `json:"product"`
	}
	if err := json.Unmarshal(resp.Data, &st); err == nil {
		a.postUI(func() {
			wasLoggedIn := a.spot.loggedIn
			a.spot.loggedIn = st.LoggedIn
			a.spot.product = st.Product
			if a.spot.loggedIn {
				if len(a.spot.playlists) == 0 && len(a.spot.liked) == 0 {
					go a.loadCachedSpotifyCatalog()
				}
				if (!wasLoggedIn || (len(a.spot.liked) == 0 && len(a.spot.playlists) == 0)) && !a.spot.syncing.Load() {
					a.syncSpotifyCatalog()
				}
			}
		})
	}
}

// syncSpotifyCatalog synchronizes playlists, albums, recent, and liked tracks sequentially.
func (a *App) syncSpotifyCatalog() {
	if a.offline || a.paths.Socket == "" || !a.spot.syncing.CompareAndSwap(false, true) {
		return
	}
	hasItems := len(a.spot.playlists) > 0 || len(a.spot.liked) > 0 || len(a.spot.albums) > 0
	if !hasItems {
		a.spot.loading = true
		a.spot.loadingMsg = T("spotify_syncing")
	}

	go func() {
		defer func() {
			a.spot.syncing.Store(false)
			a.postUI(func() {
				a.spot.loading = false
				a.spot.loadingMsg = ""
			})
		}()

		// Step 0: Preload cached catalog immediately so UI is populated without delay.
		a.loadCachedSpotifyCatalog()

		// Step 1: Load recent tracks
		if !hasItems {
			a.postUI(func() { a.spot.loadingMsg = T("spotify_loading_recent") })
		}
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"recent"}}); err == nil {
			var recent []spotifyTrack
			if err := json.Unmarshal(resp.Data, &recent); err == nil && len(recent) > 0 {
				a.postUI(func() { a.spot.recent = recent })
			}
		}

		// Step 2: Load playlists
		if !hasItems {
			a.postUI(func() { a.spot.loadingMsg = T("spotify_loading_playlists") })
		}
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"playlists"}}); err == nil {
			var pls []spotifyPlaylist
			if err := json.Unmarshal(resp.Data, &pls); err == nil && len(pls) > 0 {
				a.postUI(func() { a.spot.playlists = pls })
			}
		}

		// Step 3: Load liked songs
		if !hasItems {
			a.postUI(func() { a.spot.loadingMsg = T("spotify_loading_liked") })
		}
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked_page", "0", "50", "true"}}); err == nil {
			var res struct {
				Tracks []spotifyTrack `json:"tracks"`
				Total  int            `json:"total"`
			}
			if err := json.Unmarshal(resp.Data, &res); err == nil && len(res.Tracks) > 0 {
				a.postUI(func() {
					a.spot.likedTotal = res.Total
					a.spot.liked = res.Tracks
				})
			}
		} else if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked"}}); err == nil {
			var liked []spotifyTrack
			if err := json.Unmarshal(resp.Data, &liked); err == nil && len(liked) > 0 {
				a.postUI(func() { a.spot.liked = liked })
			}
		}

		// Step 4: Load saved albums
		if !hasItems {
			a.postUI(func() { a.spot.loadingMsg = T("spotify_loading_albums") })
		}
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"albums"}}); err == nil {
			var albums []spotifyPlaylist
			if err := json.Unmarshal(resp.Data, &albums); err == nil && len(albums) > 0 {
				for i := range albums {
					albums[i].IsAlbum = true
				}
				a.postUI(func() { a.spot.albums = albums })
			}
		}
	}()
}

// checkSpotifyTabLoad ensures that the active tab fetches its contents if empty.
func (a *App) checkSpotifyTabLoad() {
	s := &a.spot
	if !s.loggedIn || s.loading || a.offline || a.paths.Socket == "" {
		return
	}
	switch s.tab {
	case spLiked:
		if !s.likedLoaded {
			s.loading = true
			s.loadingMsg = T("spotify_loading_liked")
			go func() {
				defer a.postUI(func() {
					s.loading = false
					s.loadingMsg = ""
				})
				mergeLiked := func(newTracks []spotifyTrack, total int) {
					s.likedLoaded = true
					if total > 0 {
						s.likedTotal = total
					} else if len(newTracks) > s.likedTotal {
						s.likedTotal = len(newTracks)
					}
					have := make(map[string]bool, len(newTracks))
					for _, t := range newTracks {
						have[extractSpotifyID(t.ID)] = true
					}
					merged := append([]spotifyTrack(nil), newTracks...)
					for _, t := range s.liked {
						id := extractSpotifyID(t.ID)
						if !have[id] {
							merged = append([]spotifyTrack{t}, merged...)
							have[id] = true
						}
					}
					s.liked = merged
					a.applySpotifySort()
				}
				loaded := false
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked_page", "0", "50", "false"}}); err == nil {
					var res struct {
						Tracks []spotifyTrack `json:"tracks"`
						Total  int            `json:"total"`
					}
					if err := json.Unmarshal(resp.Data, &res); err == nil && len(res.Tracks) > 0 {
						loaded = true
						a.postUI(func() {
							mergeLiked(res.Tracks, res.Total)
						})
					}
				}
				if !loaded {
					if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_liked"}}); err == nil {
						var liked []spotifyTrack
						if err := json.Unmarshal(resp.Data, &liked); err == nil && len(liked) > 0 {
							loaded = true
							a.postUI(func() { mergeLiked(liked, len(liked)) })
						}
					}
				}
				if !loaded {
					if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked"}}); err == nil {
						var liked []spotifyTrack
						if err := json.Unmarshal(resp.Data, &liked); err == nil && len(liked) > 0 {
							loaded = true
							a.postUI(func() { mergeLiked(liked, len(liked)) })
						}
					}
				}
				if !loaded {
					a.postUI(func() { s.likedLoaded = true })
				}
			}()
		}
	case spPlaylists:
		if len(s.playlists) == 0 {
			s.loading = true
			s.loadingMsg = T("spotify_loading_playlists")
			go func() {
				defer a.postUI(func() {
					s.loading = false
					s.loadingMsg = ""
				})
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_playlists"}}); err == nil {
					var pls []spotifyPlaylist
					if err := json.Unmarshal(resp.Data, &pls); err == nil && len(pls) > 0 {
						a.postUI(func() {
							s.playlists = pls
							a.applySpotifySort()
						})
					}
				}
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"playlists"}}); err == nil {
					var pls []spotifyPlaylist
					if err := json.Unmarshal(resp.Data, &pls); err == nil && len(pls) > 0 {
						a.postUI(func() {
							s.playlists = pls
							a.applySpotifySort()
						})
					}
				}
			}()
		}
	case spAlbums:
		if len(s.albums) == 0 {
			s.loading = true
			s.loadingMsg = T("spotify_loading_albums")
			go func() {
				defer a.postUI(func() {
					s.loading = false
					s.loadingMsg = ""
				})
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_albums"}}); err == nil {
					var albums []spotifyPlaylist
					if err := json.Unmarshal(resp.Data, &albums); err == nil && len(albums) > 0 {
						for i := range albums {
							albums[i].IsAlbum = true
						}
						a.postUI(func() { s.albums = albums })
					}
				}
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"albums"}}); err == nil {
					var albums []spotifyPlaylist
					if err := json.Unmarshal(resp.Data, &albums); err == nil && len(albums) > 0 {
						for i := range albums {
							albums[i].IsAlbum = true
						}
						a.postUI(func() { s.albums = albums })
					}
				}
			}()
		}
	case spRecent:
		if len(s.recent) == 0 {
			s.loading = true
			s.loadingMsg = T("spotify_loading_recent")
			go func() {
				defer a.postUI(func() {
					s.loading = false
					s.loadingMsg = ""
				})
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"recent"}}); err == nil {
					var recent []spotifyTrack
					if err := json.Unmarshal(resp.Data, &recent); err == nil && len(recent) > 0 {
						a.postUI(func() { s.recent = recent })
					}
				}
			}()
		}
	}
}

// loadSpotifyPlaylistTracks loads tracks for a playlist in the background.
func (a *App) loadSpotifyPlaylistTracks(pl *spotifyPlaylist) {
	if a.offline || a.paths.Socket == "" || pl == nil {
		return
	}
	a.spot.loading = true
	a.spot.loadingMsg = T("loading")
	go func() {
		defer a.postUI(func() {
			a.spot.loading = false
			a.spot.loadingMsg = ""
		})
		pageCmd := "playlist_page"
		if pl.IsAlbum {
			pageCmd = "album_page"
		}
		resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{pageCmd, pl.ID, "0", "50"}})
		if err != nil {
			return
		}
		var res struct {
			Tracks []spotifyTrack `json:"tracks"`
			Total  int            `json:"total"`
		}
		if err := json.Unmarshal(resp.Data, &res); err == nil && len(res.Tracks) > 0 {
			a.postUI(func() {
				pl.Tracks = res.Tracks
				if res.Total > 0 {
					pl.TrackCount = res.Total
				} else if pl.TrackCount < len(res.Tracks) {
					pl.TrackCount = len(res.Tracks)
				}
				targetList := &a.spot.playlists
				if pl.IsAlbum {
					targetList = &a.spot.albums
				}
				for i := range *targetList {
					if (*targetList)[i].ID == pl.ID {
						(*targetList)[i].Tracks = res.Tracks
						(*targetList)[i].TrackCount = pl.TrackCount
						break
					}
				}
			})
		}
	}()
}

// startSpotifyLogin initiates the QR authentication flow with the daemon.
func (a *App) startSpotifyLogin() {
	if a.offline || a.paths.Socket == "" {
		a.flash(T("spotify_not_logged_in"))
		return
	}
	a.spot.loading = true
	go func() {
		defer a.postUI(func() { a.spot.loading = false })
		resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"login"}})
		if err != nil {
			msg := err.Error()
			a.postUI(func() { a.flash(msg) })
			return
		}
		var data struct {
			LoginURL string `json:"login_url"`
		}
		if err := json.Unmarshal(resp.Data, &data); err == nil && data.LoginURL != "" {
			loginURL := data.LoginURL
			a.postUI(func() { a.spot.loginURL = loginURL })
		}
	}()
}

// updateSpotify handles user input for the Spotify screen.
func (a *App) updateSpotify() {
	in := &a.input
	s := &a.spot

	// Periodic status check if not logged in or during login flow.
	if (!s.loggedIn || s.loginURL != "") && time.Since(s.lastCheck) > 2*time.Second {
		s.lastCheck = time.Now()
		go a.checkSpotifyStatus()
		if s.loggedIn && s.loginURL != "" {
			s.loginURL = ""
			a.flash(fmt.Sprintf(T("spotify_logged_in"), s.product))
		}
	}

	// When not logged in
	if !s.loggedIn {
		if in.Pressed(ActConfirm) {
			if s.loginURL == "" {
				a.startSpotifyLogin()
			}
		}
		if in.Pressed(ActBack) {
			if s.loginURL != "" {
				s.loginURL = ""
			} else {
				a.goBack()
			}
		}
		return
	}

	// When a playlist is currently open
	if s.openPl != nil {
		n := len(s.openPl.Tracks)
		switch {
		case in.Repeat(ActUp):
			s.openPlSel = max(0, s.openPlSel-1)
		case in.Repeat(ActDown):
			s.openPlSel = min(max(0, n-1), s.openPlSel+1)
			if a.shouldFetchNextSpotifyPage(s.openPl, s.openPlSel) {
				a.fetchNextSpotifyPage()
			}
		case in.Repeat(ActLeft):
			s.openPlSel = max(0, s.openPlSel-10)
		case in.Repeat(ActRight):
			s.openPlSel = min(max(0, n-1), s.openPlSel+10)
			if a.shouldFetchNextSpotifyPage(s.openPl, s.openPlSel) {
				a.fetchNextSpotifyPage()
			}
		case in.Pressed(ActBack):
			s.openPl = nil
		case in.Pressed(ActConfirm) && n > 0:
			tr := s.openPl.Tracks[s.openPlSel]
			collType := "playlist"
			if s.openPl.IsAlbum {
				collType = "album"
			}
			a.playSpotifyCollection(collType, s.openPl.ID, s.openPl.Tracks, s.openPlSel)
			a.addRecentSpotify(tr)
		case in.Pressed(ActMenu):
			if n > 0 {
				a.spotifyTrackMenu(s.openPl.Tracks, s.openPlSel)
			} else {
				a.openSpotifySortMenu()
			}
		}
		return
	}

	// Main tabs
	n := 0
	switch s.tab {
	case spSearch:
		n = 1 + len(s.searchRes)
	case spRecent:
		n = len(s.recent)
	case spLiked:
		n = len(s.liked)
	case spPlaylists:
		n = len(s.playlists)
	case spAlbums:
		n = len(s.albums)
	}

	sel := s.sel[s.tab]
	switch {
	case in.Pressed(ActBack):
		if s.tabFocus {
			a.goBack()
		} else {
			s.tabFocus = true
		}
	case s.tabFocus && in.Repeat(ActUp):
		s.tab = (s.tab + spCount - 1) % spCount
		a.checkSpotifyTabLoad()
	case s.tabFocus && in.Repeat(ActDown):
		s.tab = (s.tab + 1) % spCount
		a.checkSpotifyTabLoad()
	case s.tabFocus && (in.Pressed(ActConfirm) || in.Pressed(ActRight)):
		s.tabFocus = false
	case !s.tabFocus && in.Repeat(ActUp):
		s.sel[s.tab] = max(0, sel-1)
	case !s.tabFocus && in.Repeat(ActDown):
		s.sel[s.tab] = min(max(0, n-1), sel+1)
		if s.tab == spLiked {
			plLiked := &spotifyPlaylist{ID: "liked", TrackCount: s.likedTotal, Tracks: s.liked}
			if a.shouldFetchNextSpotifyPage(plLiked, s.sel[spLiked]) {
				a.fetchNextSpotifyLikedPage()
			}
		}
	case !s.tabFocus && in.Repeat(ActLeft):
		s.sel[s.tab] = max(0, sel-10)
	case !s.tabFocus && in.Repeat(ActRight):
		s.sel[s.tab] = min(max(0, n-1), sel+10)
		if s.tab == spLiked {
			plLiked := &spotifyPlaylist{ID: "liked", TrackCount: s.likedTotal, Tracks: s.liked}
			if a.shouldFetchNextSpotifyPage(plLiked, s.sel[spLiked]) {
				a.fetchNextSpotifyLikedPage()
			}
		}
	case in.Pressed(ActShuffle):
		a.spotifyAskSearch()
	case !s.tabFocus && in.Pressed(ActConfirm):
		switch s.tab {
		case spSearch:
			if sel == 0 {
				a.spotifyAskSearch()
			} else {
				idx := sel - 1
				if idx >= 0 && idx < len(s.searchRes) {
					tr := s.searchRes[idx]
					a.playSpotifyTracks(s.searchRes, idx)
					a.addRecentSpotify(tr)
				}
			}
		case spRecent:
			if n > 0 && sel < len(s.recent) {
				tr := s.recent[sel]
				a.playSpotifyTracks(s.recent, sel)
				a.addRecentSpotify(tr)
			}
		case spLiked:
			if n > 0 && sel < len(s.liked) {
				tr := s.liked[sel]
				a.playSpotifyCollection("liked", "liked", s.liked, sel)
				a.addRecentSpotify(tr)
			}
		case spPlaylists:
			if n > 0 && sel < len(s.playlists) {
				pl := s.playlists[sel]
				s.openPl = &pl
				s.openPlSel = 0
				if len(s.openPl.Tracks) == 0 {
					a.loadSpotifyPlaylistTracks(s.openPl)
				}
			}
		case spAlbums:
			if n > 0 && sel < len(s.albums) {
				pl := s.albums[sel]
				pl.IsAlbum = true
				s.openPl = &pl
				s.openPlSel = 0
				if len(s.openPl.Tracks) == 0 {
					a.loadSpotifyPlaylistTracks(s.openPl)
				}
			}
		}
	case !s.tabFocus && in.Pressed(ActMenu):
		switch s.tab {
		case spSearch:
			if sel > 0 {
				idx := sel - 1
				if idx >= 0 && idx < len(s.searchRes) {
					a.spotifyTrackMenu(s.searchRes, idx)
				}
			}
		case spRecent:
			if n > 0 && sel < len(s.recent) {
				a.spotifyTrackMenu(s.recent, sel)
			}
		case spLiked:
			if n > 0 && sel < len(s.liked) {
				a.spotifyTrackMenu(s.liked, sel)
			} else {
				a.openSpotifySortMenu()
			}
		case spPlaylists:
			if n > 0 && sel < len(s.playlists) {
				pl := s.playlists[sel]
				a.spotifyPlaylistMenu(pl)
			} else {
				a.openSpotifyPlSortMenu()
			}
		case spAlbums:
			if n > 0 && sel < len(s.albums) {
				pl := s.albums[sel]
				a.spotifyPlaylistMenu(pl)
			} else {
				a.openSpotifyPlSortMenu()
			}
		}
	}
}

// spotifyAskSearch opens the virtual keyboard to search Spotify.
func (a *App) spotifyAskSearch() {
	a.openRawPrompt(T("spotify_search"), a.spot.query, false, func(q string) {
		q = strings.TrimSpace(q)
		if q == "" {
			return
		}
		a.spot.query = q
		a.spot.tab = spSearch
		a.spot.tabFocus = false
		a.spotifySearch(q)
	})
}

// spotifySearch dispatches a search query to the daemon.
func (a *App) spotifySearch(query string) {
	if a.offline || a.paths.Socket == "" {
		return
	}
	a.spot.loading = true
	a.spot.loadingMsg = T("spotify_searching")
	go func() {
		defer a.postUI(func() {
			a.spot.loading = false
			a.spot.loadingMsg = ""
		})
		resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"search", query}})
		if err != nil {
			return
		}
		var tracks []spotifyTrack
		if err := json.Unmarshal(resp.Data, &tracks); err == nil {
			a.postUI(func() {
				a.spot.searchRes = tracks
				a.spot.sel[spSearch] = 0
			})
		}
	}()
}

// spotifyTrackMenu displays context menu for a Spotify track within its surrounding list.
func (a *App) spotifyTrackMenu(tracks []spotifyTrack, idx int) {
	if idx < 0 || idx >= len(tracks) {
		return
	}
	queue := append([]spotifyTrack(nil), tracks...)
	tr := queue[idx]
	m := &menu{title: tr.Title, subtitle: tr.Artist}
	m.items = append(m.items, menuItem{
		label: T("spotify_play"),
		icon:  "play",
		do: func() {
			if a.spot.openPl != nil {
				collType := "playlist"
				if a.spot.openPl.IsAlbum {
					collType = "album"
				}
				a.playSpotifyCollection(collType, a.spot.openPl.ID, queue, idx)
			} else if a.spot.tab == spLiked {
				a.playSpotifyCollection("liked", "liked", queue, idx)
			} else {
				a.playSpotifyTracks(queue, idx)
			}
			a.addRecentSpotify(tr)
		},
	})
	m.items = append(m.items, menuItem{
		label: T("spotify_play_next"),
		icon:  "next",
		do: func() {
			a.send("enqueue", "next", "spotify://track/"+tr.ID)
			a.flash(T("queued_next"))
		},
	})
	likeLabel := T("spotify_like")
	if a.isSpotifyTrackLiked(tr.ID) {
		likeLabel = T("spotify_unlike")
	}
	m.items = append(m.items,
		menuItem{
			label: likeLabel,
			icon:  "heart",
			do: func() {
				a.toggleSpotifyLike(tr)
			},
		},
		menuItem{
			label: T("t_addlist"),
			icon:  "list",
			do: func() {
				a.openAddToSpotifyPlaylist(tr)
			},
		},
	)
	_, downloaded := a.isSpotifyTrackDownloaded(tr)
	if downloaded {
		m.items = append(m.items, menuItem{
			label: T("spotify_downloaded"),
			icon:  "check",
			do: func() {
				a.flash(T("spotify_downloaded"))
			},
		})
	} else {
		m.items = append(m.items, menuItem{
			label: T("spotify_download"),
			icon:  "download",
			do: func() {
				a.downloadSpotifyTrack(tr)
			},
		})
	}
	if a.spot.openPl != nil && len(a.spot.openPl.Tracks) > 1 {
		pl := *a.spot.openPl
		downLabel := T("spotify_download_playlist")
		if pl.IsAlbum {
			downLabel = T("spotify_download_all")
		}
		m.items = append(m.items, menuItem{
			label: downLabel,
			icon:  "download",
			do: func() {
				a.downloadSpotifyPlaylist(pl)
			},
		})
	} else if a.spot.tab == spLiked && a.spot.openPl == nil && len(a.spot.liked) > 1 {
		m.items = append(m.items, menuItem{
			label: T("spotify_download_all"),
			icon:  "download",
			do: func() {
				if a.spot.likedTotal > len(a.spot.liked) && a.paths.Socket != "" {
					musicDir := a.ensureSpotifyMusicDir()
					go func() {
						tracks := a.spot.liked
						if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"liked"}}); err == nil {
							var fetched []spotifyTrack
							if err := json.Unmarshal(resp.Data, &fetched); err == nil && len(fetched) > 0 {
								tracks = fetched
							}
						}
						a.downloadSpotifyTracksTo(T("spotify_liked"), tracks, musicDir)
					}()
				} else {
					a.downloadSpotifyTracks(T("spotify_liked"), a.spot.liked)
				}
			},
		})
	}
	if len(a.downloadJobs()) > 0 {
		m.items = append(m.items, menuItem{label: T("ytm_jobs"), icon: "list", do: a.openDownloads})
	}
	if a.spot.openPl != nil || a.spot.tab == spLiked {
		m.items = append(m.items, menuItem{
			label: T("sm_sort", a.spotifySortLabel()),
			icon:  "list",
			do: func() {
				a.openSpotifySortMenu()
			},
		})
	}
	a.menu = m
}

// isSpotifyTrackLiked checks whether a track is in the user's liked list.
func (a *App) isSpotifyTrackLiked(id string) bool {
	id = extractSpotifyID(id)
	if id == "" {
		return false
	}
	for _, tr := range a.spot.liked {
		if extractSpotifyID(tr.ID) == id {
			return true
		}
	}
	return false
}

// toggleSpotifyLike adds or removes a track from Spotify Liked Songs.
func (a *App) toggleSpotifyLike(tr spotifyTrack) {
	tr.ID = extractSpotifyID(tr.ID)
	if tr.ID == "" {
		return
	}
	isLiked := a.isSpotifyTrackLiked(tr.ID)
	subCmd := "like"
	if isLiked {
		subCmd = "unlike"
	}

	// Update locally on the UI thread immediately for instant feedback
	if !isLiked {
		found := false
		for _, t := range a.spot.liked {
			if extractSpotifyID(t.ID) == tr.ID {
				found = true
				break
			}
		}
		if !found {
			a.spot.liked = append([]spotifyTrack{tr}, a.spot.liked...)
			a.spot.likedTotal++
		}
		a.flash(T("fav_on"))
	} else {
		filtered := a.spot.liked[:0]
		for _, t := range a.spot.liked {
			if extractSpotifyID(t.ID) != tr.ID {
				filtered = append(filtered, t)
			}
		}
		a.spot.liked = filtered
		if a.spot.likedTotal > 0 {
			a.spot.likedTotal--
		}
		a.flash(T("fav_off"))
	}

	if a.paths.Socket != "" {
		go func() {
			resp, err := control.Call(a.paths.Socket, control.Request{
				Cmd:  "spotify",
				Args: []string{subCmd, tr.ID},
			})
			if err != nil {
				a.postUI(func() {
					a.flash(err.Error())
					// Revert optimistic update
					if !isLiked {
						filtered := a.spot.liked[:0]
						for _, t := range a.spot.liked {
							if extractSpotifyID(t.ID) != tr.ID {
								filtered = append(filtered, t)
							}
						}
						a.spot.liked = filtered
						if a.spot.likedTotal > 0 {
							a.spot.likedTotal--
						}
					} else {
						a.spot.liked = append([]spotifyTrack{tr}, a.spot.liked...)
						a.spot.likedTotal++
					}
				})
				return
			}
			var m map[string]any
			_ = json.Unmarshal(resp.Data, &m)
			if b, ok := m["liked"].(bool); ok && b == isLiked {
				// Reconcile if server state differs from optimistic result
				a.postUI(func() {
					if b {
						found := false
						for _, t := range a.spot.liked {
							if extractSpotifyID(t.ID) == tr.ID {
								found = true
								break
							}
						}
						if !found {
							a.spot.liked = append([]spotifyTrack{tr}, a.spot.liked...)
							a.spot.likedTotal++
						}
					} else {
						filtered := a.spot.liked[:0]
						for _, t := range a.spot.liked {
							if extractSpotifyID(t.ID) != tr.ID {
								filtered = append(filtered, t)
							}
						}
						a.spot.liked = filtered
						if a.spot.likedTotal > 0 {
							a.spot.likedTotal--
						}
					}
				})
			}
		}()
	}
}

// openAddToSpotifyPlaylist displays Spotify playlists to add a track to.
func (a *App) openAddToSpotifyPlaylist(tr spotifyTrack) {
	if len(a.spot.playlists) == 0 && a.paths.Socket != "" {
		if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{"cached_playlists"}}); err == nil {
			var pls []spotifyPlaylist
			if err := json.Unmarshal(resp.Data, &pls); err == nil {
				a.spot.playlists = pls
			}
		}
	}
	title := tr.Title
	if title == "" {
		title = "Spotify Track"
	}
	m := &menu{title: T("t_addlist"), subtitle: title}
	for _, pl := range a.spot.playlists {
		if pl.IsAlbum {
			continue
		}
		targetPl := pl
		m.items = append(m.items, menuItem{
			label: targetPl.Name,
			icon:  "list",
			do: func() {
				a.addSpotifyTrackToPlaylist(targetPl, tr)
			},
		})
	}
	a.menu = m
}

// addSpotifyTrackToPlaylist adds a Spotify track to a specified Spotify playlist.
func (a *App) addSpotifyTrackToPlaylist(pl spotifyPlaylist, tr spotifyTrack) {
	if pl.ID == "" || tr.ID == "" {
		return
	}
	if a.paths.Socket != "" {
		go func() {
			_, err := control.Call(a.paths.Socket, control.Request{
				Cmd:  "spotify",
				Args: []string{"add_to_playlist", pl.ID, tr.ID},
			})
			if err != nil {
				a.postUI(func() { a.flash(err.Error()) })
				return
			}
			a.postUI(func() {
				for i := range a.spot.playlists {
					if a.spot.playlists[i].ID == pl.ID {
						a.spot.playlists[i].TrackCount++
						a.spot.playlists[i].Tracks = append(a.spot.playlists[i].Tracks, tr)
						break
					}
				}
				a.flash(T("pl_added", tr.Title, pl.Name))
			})
		}()
	} else {
		for i := range a.spot.playlists {
			if a.spot.playlists[i].ID == pl.ID {
				a.spot.playlists[i].TrackCount++
				a.spot.playlists[i].Tracks = append(a.spot.playlists[i].Tracks, tr)
				break
			}
		}
		a.flash(T("pl_added", tr.Title, pl.Name))
	}
}

// cycleShuffle cycles shuffle mode: Off -> Shuffle -> Smart Shuffle (if Spotify available) -> Off.
func (a *App) cycleShuffle() {
	if !a.status.Shuffle {
		a.send("shuffle", "on")
		a.send("spotify", "smart_shuffle", "off")
		a.status.Shuffle = true
		a.status.SpotifySmartShuffle = false
		a.flash(T("set_shuffle") + ": " + T("on"))
		return
	}
	if a.status.Shuffle && !a.status.SpotifySmartShuffle {
		hasSpotify := isSpotifyPath(a.playingSong.Path)
		if !hasSpotify {
			for _, p := range a.queue() {
				if isSpotifyPath(p) {
					hasSpotify = true
					break
				}
			}
		}
		if hasSpotify || a.spot.loggedIn {
			a.send("spotify", "smart_shuffle", "on")
			a.status.SpotifySmartShuffle = true
			a.flash(T("spotify_smart_shuffle"))
			return
		}
		a.send("shuffle", "off")
		a.send("spotify", "smart_shuffle", "off")
		a.status.Shuffle = false
		a.status.SpotifySmartShuffle = false
		a.flash(T("set_shuffle") + ": " + T("off"))
		return
	}
	a.send("spotify", "smart_shuffle", "off")
	a.send("shuffle", "off")
	a.status.Shuffle = false
	a.status.SpotifySmartShuffle = false
	a.flash(T("set_shuffle") + ": " + T("off"))
}

// shuffleModeLabel returns the display string for current shuffle mode.
func (a *App) shuffleModeLabel() string {
	if a.status.SpotifySmartShuffle {
		return T("spotify_smart_shuffle")
	}
	if a.status.Shuffle {
		return T("on")
	}
	return T("off")
}

// spotifyPlaylistMenu displays context menu for a playlist.
func (a *App) spotifyPlaylistMenu(pl spotifyPlaylist) {
	count := pl.TrackCount
	if len(pl.Tracks) > count {
		count = len(pl.Tracks)
	}
	m := &menu{title: pl.Name, subtitle: N("n_songs", count)}
	m.items = append(m.items, menuItem{
		label: T("spotify_play"),
		icon:  "play",
		do: func() {
			collType := "playlist"
			if pl.IsAlbum {
				collType = "album"
			}
			if len(pl.Tracks) > 0 {
				a.playSpotifyCollection(collType, pl.ID, pl.Tracks, 0)
				return
			}
			if a.offline || a.paths.Socket == "" {
				return
			}
			go func() {
				subCmd := spotifySubcommandForPlaylist(pl)
				var tracks []spotifyTrack
				if resp, err := control.Call(a.paths.Socket, control.Request{Cmd: "spotify", Args: []string{subCmd, pl.ID}}); err == nil {
					_ = json.Unmarshal(resp.Data, &tracks)
				}
				if len(tracks) > 0 {
					a.postUI(func() { a.playSpotifyCollection(collType, pl.ID, tracks, 0) })
				}
			}()
		},
	})
	downLabel := T("spotify_download_playlist")
	if pl.IsAlbum {
		downLabel = T("spotify_download_all")
	}
	m.items = append(m.items, menuItem{
		label: downLabel,
		icon:  "download",
		do: func() {
			a.downloadSpotifyPlaylist(pl)
		},
	})
	if len(a.downloadJobs()) > 0 {
		m.items = append(m.items, menuItem{label: T("ytm_jobs"), icon: "list", do: a.openDownloads})
	}
	m.items = append(m.items, menuItem{
		label: T("sm_sort", a.spotifyPlSortLabel()),
		icon:  "list",
		do: func() {
			a.openSpotifyPlSortMenu()
		},
	})
	a.menu = m
}

// drawSpotify renders the Spotify screen.
func (a *App) drawSpotify(dst *ebiten.Image) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	s := &a.spot
	x0, top := a.contentX(), topH+8

	// When not logged in, display login prompt or QR code.
	if !s.loggedIn {
		a.drawTopBar(dst, T("scr_spotify"))
		if s.loginURL != "" {
			a.drawSpotifyQR(dst, s.loginURL)
		} else {
			a.drawSpotifyLoginPrompt(dst)
		}
		hints := []hint{{"A", T("spotify_login_btn")}, {"B", T("h_back")}}
		a.drawHints(dst, hints)
		return
	}

	// Top bar title
	title := T("scr_spotify") + " · " + T(spotifyTabKeys[s.tab])
	if s.tab == spLiked && s.likedTotal > 0 {
		if len(s.liked) < s.likedTotal {
			title = fmt.Sprintf("%s (%d/%d)", title, len(s.liked), s.likedTotal)
		} else {
			title = fmt.Sprintf("%s (%d)", title, s.likedTotal)
		}
	}
	if s.openPl != nil {
		title = T("scr_spotify") + " · " + s.openPl.Name
	}

	// Side tabs on wide screens
	if w >= 320 {
		for i := 0; i < spCount; i++ {
			y := topH + 8 + i*16
			tabName := T(spotifyTabKeys[i])
			idx := i
			a.tapZone(margin-3, y-4, 72, 13, func() {
				s.openPl = nil
				s.tab = idx
				s.tabFocus = false
				a.checkSpotifyTabLoad()
			})
			switch {
			case i == s.tab && s.tabFocus:
				panel(dst, margin-3, y-4, 72, 13, th.Accent)
				drawText(dst, tabName, SmallFace, margin+2, y, th.Bg)
			case i == s.tab:
				panel(dst, margin-3, y-4, 72, 13, mix(th.Bg2, th.Accent, 0.35))
				drawText(dst, tabName, SmallFace, margin+2, y, th.Text)
			default:
				drawText(dst, tabName, SmallFace, margin+2, y, th.Dim)
			}
		}
		rect(dst, margin+75, topH+5, 1, h-topH-bottomH-44, withAlpha(th.Dim, 0x40))
	}

	cw := w - x0 - margin
	bottom := h - bottomH - 36
	const rowH = 22

	hasItems := false
	if s.openPl != nil {
		hasItems = len(s.openPl.Tracks) > 0
	} else {
		switch s.tab {
		case spSearch:
			hasItems = len(s.searchRes) > 0
		case spRecent:
			hasItems = len(s.recent) > 0
		case spLiked:
			hasItems = len(s.liked) > 0
		case spPlaylists:
			hasItems = len(s.playlists) > 0
		case spAlbums:
			hasItems = len(s.albums) > 0
		}
	}

	// Loading banner: only shown when items are already present on screen
	if (s.loading || s.syncing.Load()) && hasItems {
		msg := s.loadingMsg
		if msg == "" {
			msg = T("loading")
		}
		bw := textWidth(msg, SmallFace) + 24
		bx := x0 + max(0, (cw-bw)/2)
		by := top - 2
		panel(dst, bx, by, bw, 13, mix(th.Bg2, spotGreen, 0.4))
		rect(dst, bx, by, 2, 13, spotGreen)
		drawIcon(dst, fmt.Sprintf("hourglass%d", int(a.t/15)%3), bx+4, by+3, spotGreen)
		drawText(dst, msg, SmallFace, bx+14, by+3, th.Text)
		top += 16
	}

	var hints []hint
	if s.openPl != nil {
		pl := s.openPl
		a.tapZone(x0-3, top-4, cw+6, 13, func() { s.openPl = nil })
		header := "< " + pl.Name
		if pl.TrackCount > len(pl.Tracks) {
			header = fmt.Sprintf("< %s (%d/%d)", pl.Name, len(pl.Tracks), pl.TrackCount)
		} else if pl.TrackCount > 0 {
			header = fmt.Sprintf("< %s (%d)", pl.Name, pl.TrackCount)
		}
		drawTextW(dst, header, SmallFace, x0, top, cw, th.Accent2)
		top += 12

		rows := max(1, (bottom-top)/rowH)
		n := len(pl.Tracks)
		first, off, clip := a.touchList(dst, &s.openPlScr, &s.openPlSel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		if a.shouldFetchNextSpotifyPage(s.openPl, s.openPlSel) {
			a.fetchNextSpotifyPage()
		}
		whole := dst
		dst = clip
		for i := 0; i <= rows && first+i < n; i++ {
			tr := pl.Tracks[first+i]
			y := top + 3 + i*rowH - off
			if first+i == s.openPlSel {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			dlPath, downloaded := a.cachedSpotifyTrackDownloaded(tr)
			isCurrent := (isSpotifyPath(a.playingSong.Path) && extractSpotifyID(a.playingSong.Path) == tr.ID) || (downloaded && dlPath != "" && a.playingSong.Path == dlPath)
			icCol := th.Dim
			if isCurrent {
				icCol = spotGreen
			}
			drawIcon(dst, "spotify", x0, y, icCol)
			textW := cw - 12
			if downloaded {
				drawIcon(dst, "check", x0+cw-10, y, spotGreen)
				textW -= 12
			}
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, textW, th.Text)
			sub := tr.Artist
			if tr.Duration > 0 {
				sub += fmt.Sprintf(" · %s", fmtDur(spotifyDurSec(tr.Duration)))
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, textW, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)
		if n == 0 {
			if s.loading {
				msg := T("loading")
				hgIcon := fmt.Sprintf("hourglass%d", int(a.t/15)%3)
				tw := textWidth(msg, SmallFace)
				drawIcon(dst, hgIcon, x0+max(0, (cw-tw-16)/2), top+30, spotGreen)
				drawText(dst, msg, SmallFace, x0+max(0, (cw-tw-16)/2)+14, top+30, th.Dim)
			} else {
				msg := T("spotify_empty")
				drawText(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, th.Dim)
			}
		}
		hints = []hint{{"A", T("h_play")}, {"START", T("h_more")}, {"B", T("h_back")}}
	} else if s.tab == spSearch {
		// Library-style search tab
		// Top search box
		panel(dst, x0-3, top-4, cw+6, 16, withAlpha(th.Bg2, 0xd0))
		if s.sel[spSearch] == 0 && !s.tabFocus {
			panel(dst, x0-3, top-4, cw+6, 16, mix(th.Bg2, th.Accent, 0.35))
			rect(dst, x0-3, top-4, 2, 16, th.Accent)
		}
		drawIcon(dst, "search", x0+1, top-1, th.Dim)
		sq := s.query
		if sq == "" {
			sq = T("spotify_search") + "..."
			drawText(dst, sq, SmallFace, x0+14, top, th.Dim)
		} else {
			drawText(dst, sq, SmallFace, x0+14, top, th.Text)
		}
		a.tapZone(x0-3, top-4, cw+6, 16, func() {
			s.sel[spSearch] = 0
			s.tabFocus = false
			a.spotifyAskSearch()
		})
		top += 18

		rows := max(1, (bottom-top)/rowH)
		n := len(s.searchRes)
		itemSel := max(0, s.sel[spSearch]-1)

		first, off, clip := a.touchList(dst, &s.scr[spSearch], &itemSel, n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			s.sel[spSearch] = itemSel + 1
			a.input.inject(ActConfirm)
		})
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			tr := s.searchRes[first+i]
			y := top + 3 + i*rowH - off
			if first+i+1 == s.sel[spSearch] && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}
			dlPath, downloaded := a.cachedSpotifyTrackDownloaded(tr)
			isCurrent := (isSpotifyPath(a.playingSong.Path) && extractSpotifyID(a.playingSong.Path) == tr.ID) || (downloaded && dlPath != "" && a.playingSong.Path == dlPath)
			icCol := th.Dim
			if isCurrent {
				icCol = spotGreen
			}
			drawIcon(dst, "spotify", x0, y, icCol)
			textW := cw - 12
			if downloaded {
				drawIcon(dst, "check", x0+cw-10, y, spotGreen)
				textW -= 12
			}
			drawTextW(dst, tr.Title, SmallFace, x0+12, y, textW, th.Text)
			sub := tr.Artist
			if tr.Duration > 0 {
				sub += fmt.Sprintf(" · %s", fmtDur(spotifyDurSec(tr.Duration)))
			}
			drawTextW(dst, sub, SmallFace, x0+12, y+9, textW, th.Dim)
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 {
			if s.loading {
				msg := T("loading")
				hgIcon := fmt.Sprintf("hourglass%d", int(a.t/15)%3)
				tw := textWidth(msg, SmallFace)
				drawIcon(dst, hgIcon, x0+max(0, (cw-tw-16)/2), top+30, spotGreen)
				drawText(dst, msg, SmallFace, x0+max(0, (cw-tw-16)/2)+14, top+30, th.Dim)
			} else {
				msg := T("spotify_empty")
				if s.query == "" {
					msg = T("spotify_search") + " (A / X)"
				}
				drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
			}
		}
		hints = []hint{{"A", T("h_play")}, {"X", T("spotify_search")}, {"START", T("h_more")}, {"B", T("h_back")}}
	} else {
		// Render tracks or playlists/albums list
		var tracks []spotifyTrack
		var playlists []spotifyPlaylist
		isTracks := true

		switch s.tab {
		case spRecent:
			tracks = s.recent
		case spLiked:
			tracks = s.liked
		case spPlaylists:
			playlists = s.playlists
			isTracks = false
		case spAlbums:
			playlists = s.albums
			isTracks = false
		}

		rows := max(1, (bottom-top)/rowH)
		n := len(tracks)
		if !isTracks {
			n = len(playlists)
		}

		first, off, clip := a.touchList(dst, &s.scr[s.tab], &s.sel[s.tab], n, x0-3, top-1, cw+6, bottom-top, rowH, func() {
			a.input.inject(ActConfirm)
		})
		if s.tab == spLiked {
			plLiked := &spotifyPlaylist{ID: "liked", TrackCount: s.likedTotal, Tracks: s.liked}
			if a.shouldFetchNextSpotifyPage(plLiked, s.sel[spLiked]) {
				a.fetchNextSpotifyLikedPage()
			}
		}
		whole := dst
		dst = clip

		for i := 0; i <= rows && first+i < n; i++ {
			y := top + 3 + i*rowH - off
			if first+i == s.sel[s.tab] && !s.tabFocus {
				panel(dst, x0-3, y-3, cw+6, rowH-2, mix(th.Bg2, th.Accent, 0.35))
				rect(dst, x0-3, y-3, 2, rowH-2, th.Accent)
			}

			if isTracks {
				tr := tracks[first+i]
				dlPath, downloaded := a.cachedSpotifyTrackDownloaded(tr)
				isCurrent := (isSpotifyPath(a.playingSong.Path) && extractSpotifyID(a.playingSong.Path) == tr.ID) || (downloaded && dlPath != "" && a.playingSong.Path == dlPath)
				icCol := th.Dim
				if isCurrent {
					icCol = spotGreen
				}
				drawIcon(dst, "spotify", x0, y, icCol)
				textW := cw - 12
				if downloaded {
					drawIcon(dst, "check", x0+cw-10, y, spotGreen)
					textW -= 12
				}
				drawTextW(dst, tr.Title, SmallFace, x0+12, y, textW, th.Text)
				sub := tr.Artist
				if tr.Duration > 0 {
					sub += fmt.Sprintf(" · %s", fmtDur(spotifyDurSec(tr.Duration)))
				}
				drawTextW(dst, sub, SmallFace, x0+12, y+9, textW, th.Dim)
			} else {
				pl := playlists[first+i]
				drawIcon(dst, "list", x0, y, th.Accent2)
				count := pl.TrackCount
				if len(pl.Tracks) > count {
					count = len(pl.Tracks)
				}
				sub := N("n_songs", count)
				if len(pl.Tracks) > 0 {
					downCount, complete := a.playlistDownloadCount(pl)
					if complete && downCount == count && downCount > 0 {
						drawIcon(dst, "check", x0+cw-10, y, spotGreen)
						sub += " · " + T("spotify_downloaded")
					} else if downCount > 0 {
						sub += fmt.Sprintf(" · %d/%d", downCount, count)
					}
				}
				drawTextW(dst, pl.Name, SmallFace, x0+12, y, cw-12, th.Text)
				drawTextW(dst, sub, SmallFace, x0+12, y+9, cw-12, th.Dim)
			}
		}
		dst = whole
		a.scrollBar(dst, x0+cw+4, top, bottom-top, first, rows, n)

		if n == 0 {
			if s.loading || s.syncing.Load() {
				msg := s.loadingMsg
				if msg == "" {
					msg = T("loading")
				}
				hgIcon := fmt.Sprintf("hourglass%d", int(a.t/15)%3)
				tw := textWidth(msg, SmallFace)
				drawIcon(dst, hgIcon, x0+max(0, (cw-tw-16)/2), top+30, spotGreen)
				drawText(dst, msg, SmallFace, x0+max(0, (cw-tw-16)/2)+14, top+30, th.Text)
			} else {
				msg := T("spotify_empty")
				if s.tab == spRecent {
					msg = T("spotify_no_recent")
				}
				drawTextW(dst, msg, SmallFace, x0+max(0, (cw-textWidth(msg, SmallFace))/2), top+30, cw, th.Dim)
			}
		}

		hints = []hint{{"A", T("h_play")}, {"X", T("spotify_search")}, {"START", T("h_more")}, {"B", T("h_back")}}
	}

	a.drawMiniPlayer(dst)
	a.drawTopBar(dst, title)
	a.drawHints(dst, hints)
}

// drawSpotifyLoginPrompt renders centered login prompt.
func (a *App) drawSpotifyLoginPrompt(dst *ebiten.Image) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	cx := w / 2
	cy := topH + (h-topH-bottomH)/2

	drawIcon(dst, "spotify", cx-5, cy-48, spotGreen)
	title := "Spotify"
	drawText(dst, title, BigFace, cx-textWidth(title, BigFace)/2, cy-32, th.Text)

	desc := T("spotify_login_prompt")
	tw := textWidth(desc, SmallFace)
	if tw <= w-40 {
		drawText(dst, desc, SmallFace, cx-tw/2, cy-8, th.Dim)
	} else {
		drawTextW(dst, desc, SmallFace, cx-(w-40)/2, cy-8, w-40, th.Dim)
	}

	// Login button
	btnW, btnH := 160, 24
	btnX := cx - btnW/2
	btnY := cy + 16
	panel(dst, btnX, btnY, btnW, btnH, mix(th.Bg2, spotGreen, 0.4))
	rect(dst, btnX, btnY, 2, btnH, spotGreen)
	btnText := T("spotify_login_btn")
	drawText(dst, btnText, SmallFace, cx-textWidth(btnText, SmallFace)/2, btnY+7, th.Text)
}

// drawSpotifyQR renders QR code for OAuth2 PKCE login.
func (a *App) drawSpotifyQR(dst *ebiten.Image, urlStr string) {
	th := a.theme
	w, h := dst.Bounds().Dx(), dst.Bounds().Dy()
	rect(dst, 0, 0, w, h, withAlpha(th.Bg, 0xd0))

	qr := qrFor(urlStr)
	if qr == nil {
		return
	}

	n := qr.Bounds().Dx()
	room := h - topH - bottomH - 40
	s := max(1, min(room/n, 8))
	if room >= 2*n {
		s = max(2, min(room/n, 8))
	}
	size := n * s

	displayURL := urlStr
	if len(displayURL) > 40 {
		displayURL = displayURL[:37] + "..."
	}

	pw := max(size, textWidth(displayURL, SmallFace)) + 24
	ph := size + 44
	px, py := (w-pw)/2, topH+(h-topH-bottomH-ph)/2

	panel(dst, px, py, pw, ph, th.Bg2)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(s), float64(s))
	op.GeoM.Translate(float64((w-size)/2), float64(py+8))
	dst.DrawImage(qr, op)

	scanText := T("spotify_login_scan")
	drawText(dst, scanText, SmallFace, (w-textWidth(scanText, SmallFace))/2, py+size+14, spotGreen)
	drawText(dst, displayURL, SmallFace, (w-textWidth(displayURL, SmallFace))/2, py+size+26, th.Dim)
}
