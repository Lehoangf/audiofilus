package ui

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/juanvillacortac/audiofilus/internal/lyrics"
	"github.com/juanvillacortac/audiofilus/internal/ytm"
)

// filterMissingLyrics devuelve únicamente las canciones locales que aún no tienen
// letras asociadas (.lrc, .txt o etiquetas).
func filterMissingLyrics(songs []Song) []Song {
	var missing []Song
	seen := map[string]bool{}
	for _, s := range songs {
		if s.Path == "" || seen[s.Path] || !lyrics.IsLocal(s.Path) {
			continue
		}
		seen[s.Path] = true
		if !lyrics.HasLyrics(s.Path) {
			missing = append(missing, s)
		}
	}
	return missing
}

// fetchMissingLyrics encola la descarga en segundo plano de letras faltantes
// para la lista de canciones dada, mostrando el progreso en Descargas.
func (a *App) fetchMissingLyrics(songs []Song, targetTitle string) {
	missing := filterMissingLyrics(songs)
	if len(missing) == 0 {
		a.flash(T("lyrics_already_complete"))
		return
	}
	if a.cfg.UI.LyricsOffline || a.offline {
		a.flash(T("lyrics_offline_disabled"))
		return
	}
	fd := a.lyricsFinder()
	if fd == nil || fd.Web == nil {
		a.flash(T("lyrics_offline_disabled"))
		return
	}

	title := targetTitle
	if title == "" {
		title = T("lyrics_download_missing")
	}

	var fetched atomic.Int32
	total := int64(len(missing))

	a.metaJob(T("lyrics_download_missing"), title, func(ctx context.Context, progress func(done, total int64)) error {
		progress(0, total)
		for i, s := range missing {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			progress(int64(i), total)
			if i > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(120 * time.Millisecond):
				}
			}

			q := lyrics.Query{
				Title:  s.Title,
				Artist: s.Artist,
				Album:  s.Album,
			}
			if ok, err := lyrics.FetchAndSaveLRC(ctx, fd.Web, fd.Store, q, s.Path); err == nil && ok {
				fetched.Add(1)
			}
		}
		progress(total, total)
		return nil
	}, func() {
		got := int(fetched.Load())
		if got > 0 {
			a.flash(T("lyrics_fetched_count", got, len(missing)))
		} else {
			a.flash(T("lyrics_fetch_none"))
		}
	})
}

// fetchCurrentSongLyrics busca y guarda la letra para la canción en reproducción.
func (a *App) fetchCurrentSongLyrics() {
	s := a.playingSong
	if s.Path == "" || !lyrics.IsLocal(s.Path) {
		return
	}
	if lyrics.HasLyrics(s.Path) {
		a.flash(T("lyrics_already_complete"))
		return
	}
	a.fetchMissingLyrics([]Song{s}, orDefault(s.Title, filepath.Base(s.Path)))
}

// fetchLibraryMissingLyrics busca y descarga letras para toda la biblioteca.
func (a *App) fetchLibraryMissingLyrics() {
	if a.lib == nil || len(a.lib.Songs) == 0 {
		a.flash(T("empty"))
		return
	}
	a.fetchMissingLyrics(a.lib.Songs, T("sec_library"))
}

// autoFetchLyrics inicia la descarga en segundo plano de la letra (.lrc) tras
// completar la descarga de una pista.
func (a *App) autoFetchLyrics(audioPath string, tr ytm.Track) {
	if !lyrics.IsLocal(audioPath) || lyrics.HasLyrics(audioPath) {
		return
	}
	if a.cfg.UI.LyricsOffline || a.offline {
		return
	}
	go a.autoFetchLyricsSync(audioPath, tr)
}

// autoFetchLyricsSync realiza la búsqueda síncrona en LRCLIB y guardado de .lrc.
func (a *App) autoFetchLyricsSync(audioPath string, tr ytm.Track) {
	if !lyrics.IsLocal(audioPath) || lyrics.HasLyrics(audioPath) {
		return
	}
	fd := a.lyricsFinder()
	if fd == nil || fd.Web == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	artist := ""
	if len(tr.Artists) > 0 {
		artist = strings.Join(tr.Artists, ", ")
	}
	q := lyrics.Query{
		Title:    tr.Title,
		Artist:   artist,
		Album:    tr.Album,
		Duration: parseDurationSec(tr.Duration),
	}
	_, _ = lyrics.FetchAndSaveLRC(ctx, fd.Web, fd.Store, q, audioPath)
}

// parseDurationSec convierte "mm:ss" o "hh:mm:ss" a segundos en float64.
func parseDurationSec(s string) float64 {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) == 2 {
		m, _ := strconv.Atoi(parts[0])
		sec, _ := strconv.Atoi(parts[1])
		return float64(m*60 + sec)
	}
	if len(parts) == 3 {
		h, _ := strconv.Atoi(parts[0])
		m, _ := strconv.Atoi(parts[1])
		sec, _ := strconv.Atoi(parts[2])
		return float64(h*3600 + m*60 + sec)
	}
	return 0
}
