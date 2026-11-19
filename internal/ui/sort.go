package ui

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// sortPreferences guarda los modos de ordenación activos para la biblioteca y Spotify.
type sortPreferences struct {
	songs   int // 0: ruta/defecto, 1: título A-Z, 2: título Z-A, 3: artista A-Z, 4: álbum A-Z
	albums  int // 0: artista A-Z, 1: título A-Z, 2: título Z-A, 3: canciones desc
	artists int // 0: nombre A-Z, 1: nombre Z-A, 2: canciones desc, 3: álbumes desc
	lists   int // 0: nombre A-Z, 1: nombre Z-A, 2: canciones desc
	album   int // 0: pista def, 1: título A-Z, 2: título Z-A
	pl      int // 0: orden def, 1: título A-Z, 2: artista A-Z
	folder  int // 0: nombre A-Z, 1: nombre Z-A

	spotLiked  int // 0: def, 1: título A-Z, 2: título Z-A, 3: artista A-Z, 4: duración desc
	spotPls    int // 0: def, 1: nombre A-Z, 2: nombre Z-A, 3: canciones desc
	spotAlbums int // 0: def, 1: álbum A-Z, 2: artista A-Z, 3: canciones desc
	spotTracks int // 0: def, 1: título A-Z, 2: artista A-Z, 3: duración desc
}

// canSortView comprueba si la vista actual de la biblioteca admite ordenación.
func (a *App) canSortView(v *view) bool {
	if v == nil {
		return false
	}
	switch v.kind {
	case vSongs, vAlbums, vArtists, vPlaylists, vAlbum, vPlaylist, vFolder:
		return true
	}
	return false
}

// libSortLabel devuelve el texto del modo de ordenación actual para una vista.
func (a *App) libSortLabel(v *view) string {
	if v == nil {
		return ""
	}
	sp := &a.sortState
	switch v.kind {
	case vSongs:
		switch sp.songs {
		case 1:
			return T("smf_title") + " (A-Z)"
		case 2:
			return T("smf_title") + " (Z-A)"
		case 3:
			return T("smf_artist") + " (A-Z)"
		case 4:
			return T("smf_album") + " (A-Z)"
		default:
			return T("splash_col_default")
		}
	case vAlbums:
		switch sp.albums {
		case 1:
			return T("smf_album") + " (A-Z)"
		case 2:
			return T("smf_album") + " (Z-A)"
		case 3:
			return fmt.Sprintf("%s ↓", T("smf_track"))
		default:
			return T("smf_artist") + " (A-Z)"
		}
	case vArtists:
		switch sp.artists {
		case 1:
			return T("smf_artist") + " (Z-A)"
		case 2:
			return fmt.Sprintf("%s ↓", T("smf_track"))
		case 3:
			return fmt.Sprintf("%s ↓", T("smf_album"))
		default:
			return T("smf_artist") + " (A-Z)"
		}
	case vPlaylists:
		switch sp.lists {
		case 1:
			return T("smf_title") + " (Z-A)"
		case 2:
			return fmt.Sprintf("%s ↓", T("smf_track"))
		default:
			return T("smf_title") + " (A-Z)"
		}
	case vAlbum:
		switch sp.album {
		case 1:
			return T("smf_title") + " (A-Z)"
		case 2:
			return T("smf_title") + " (Z-A)"
		default:
			return T("smf_track")
		}
	case vPlaylist:
		switch sp.pl {
		case 1:
			return T("smf_title") + " (A-Z)"
		case 2:
			return T("smf_artist") + " (A-Z)"
		default:
			return T("splash_col_default")
		}
	case vFolder:
		switch sp.folder {
		case 1:
			return "Z-A"
		default:
			return "A-Z"
		}
	}
	return ""
}

// applyLibSort aplica el criterio de ordenación guardado a la vista.
func (a *App) applyLibSort(v *view) {
	if v == nil {
		return
	}
	sp := &a.sortState
	switch v.kind {
	case vSongs:
		switch sp.songs {
		case 1:
			sort.SliceStable(v.songs, func(i, j int) bool {
				return strings.ToLower(v.songs[i].Title) < strings.ToLower(v.songs[j].Title)
			})
		case 2:
			sort.SliceStable(v.songs, func(i, j int) bool {
				return strings.ToLower(v.songs[i].Title) > strings.ToLower(v.songs[j].Title)
			})
		case 3:
			sort.SliceStable(v.songs, func(i, j int) bool {
				if !strings.EqualFold(v.songs[i].Artist, v.songs[j].Artist) {
					return strings.ToLower(v.songs[i].Artist) < strings.ToLower(v.songs[j].Artist)
				}
				return strings.ToLower(v.songs[i].Title) < strings.ToLower(v.songs[j].Title)
			})
		case 4:
			sort.SliceStable(v.songs, func(i, j int) bool {
				if !strings.EqualFold(v.songs[i].Album, v.songs[j].Album) {
					return strings.ToLower(v.songs[i].Album) < strings.ToLower(v.songs[j].Album)
				}
				return v.songs[i].Track < v.songs[j].Track
			})
		default:
			if a.lib != nil {
				v.songs = append([]Song(nil), a.lib.Songs...)
			}
		}
	case vAlbums:
		switch sp.albums {
		case 1:
			sort.SliceStable(v.albums, func(i, j int) bool {
				return strings.ToLower(v.albums[i].Title) < strings.ToLower(v.albums[j].Title)
			})
		case 2:
			sort.SliceStable(v.albums, func(i, j int) bool {
				return strings.ToLower(v.albums[i].Title) > strings.ToLower(v.albums[j].Title)
			})
		case 3:
			sort.SliceStable(v.albums, func(i, j int) bool {
				return len(v.albums[i].Songs) > len(v.albums[j].Songs)
			})
		default:
			sort.SliceStable(v.albums, func(i, j int) bool {
				if !strings.EqualFold(v.albums[i].Artist, v.albums[j].Artist) {
					return strings.ToLower(v.albums[i].Artist) < strings.ToLower(v.albums[j].Artist)
				}
				return strings.ToLower(v.albums[i].Title) < strings.ToLower(v.albums[j].Title)
			})
		}
	case vArtists:
		switch sp.artists {
		case 1:
			sort.SliceStable(v.artists, func(i, j int) bool {
				return strings.ToLower(v.artists[i].Name) > strings.ToLower(v.artists[j].Name)
			})
		case 2:
			sort.SliceStable(v.artists, func(i, j int) bool {
				return v.artists[i].Songs > v.artists[j].Songs
			})
		case 3:
			sort.SliceStable(v.artists, func(i, j int) bool {
				return len(v.artists[i].Albums) > len(v.artists[j].Albums)
			})
		default:
			sort.SliceStable(v.artists, func(i, j int) bool {
				return strings.ToLower(v.artists[i].Name) < strings.ToLower(v.artists[j].Name)
			})
		}
	case vPlaylists:
		switch sp.lists {
		case 1:
			sort.SliceStable(v.playlists, func(i, j int) bool {
				return strings.ToLower(v.playlists[i].Name) > strings.ToLower(v.playlists[j].Name)
			})
		case 2:
			sort.SliceStable(v.playlists, func(i, j int) bool {
				return len(v.playlists[i].Songs) > len(v.playlists[j].Songs)
			})
		default:
			sort.SliceStable(v.playlists, func(i, j int) bool {
				return strings.ToLower(v.playlists[i].Name) < strings.ToLower(v.playlists[j].Name)
			})
		}
	case vAlbum:
		switch sp.album {
		case 1:
			sort.SliceStable(v.songs, func(i, j int) bool {
				return strings.ToLower(v.songs[i].Title) < strings.ToLower(v.songs[j].Title)
			})
		case 2:
			sort.SliceStable(v.songs, func(i, j int) bool {
				return strings.ToLower(v.songs[i].Title) > strings.ToLower(v.songs[j].Title)
			})
		default:
			if v.album != nil {
				v.songs = append([]Song(nil), v.album.Songs...)
				sort.SliceStable(v.songs, func(i, j int) bool { return v.songs[i].Track < v.songs[j].Track })
			}
		}
	case vPlaylist:
		switch sp.pl {
		case 1:
			sort.SliceStable(v.songs, func(i, j int) bool {
				return strings.ToLower(v.songs[i].Title) < strings.ToLower(v.songs[j].Title)
			})
		case 2:
			sort.SliceStable(v.songs, func(i, j int) bool {
				if !strings.EqualFold(v.songs[i].Artist, v.songs[j].Artist) {
					return strings.ToLower(v.songs[i].Artist) < strings.ToLower(v.songs[j].Artist)
				}
				return strings.ToLower(v.songs[i].Title) < strings.ToLower(v.songs[j].Title)
			})
		default:
			if v.pl != nil {
				v.songs = append([]Song(nil), v.pl.Songs...)
			}
		}
	case vFolder:
		switch sp.folder {
		case 1:
			sort.SliceStable(v.dirs, func(i, j int) bool { return strings.ToLower(v.dirs[i]) > strings.ToLower(v.dirs[j]) })
			sort.SliceStable(v.songs, func(i, j int) bool { return strings.ToLower(v.songs[i].Path) > strings.ToLower(v.songs[j].Path) })
		default:
			sort.SliceStable(v.dirs, func(i, j int) bool { return strings.ToLower(v.dirs[i]) < strings.ToLower(v.dirs[j]) })
			sort.SliceStable(v.songs, func(i, j int) bool { return strings.ToLower(v.songs[i].Path) < strings.ToLower(v.songs[j].Path) })
		}
	}
}

// openLibSortMenu despliega el menú de selección de ordenación para la biblioteca.
func (a *App) openLibSortMenu(v *view) {
	if v == nil {
		return
	}
	var items []menuItem
	addOpt := func(val int, label string) {
		items = append(items, menuItem{
			label: label,
			icon:  "list",
			do: func() {
				switch v.kind {
				case vSongs:
					a.sortState.songs = val
				case vAlbums:
					a.sortState.albums = val
				case vArtists:
					a.sortState.artists = val
				case vPlaylists:
					a.sortState.lists = val
				case vAlbum:
					a.sortState.album = val
				case vPlaylist:
					a.sortState.pl = val
				case vFolder:
					a.sortState.folder = val
				}
				a.applyLibSort(v)
				v.sel = 0
				a.flash(T("sm_sort", a.libSortLabel(v)))
			},
		})
	}

	switch v.kind {
	case vSongs:
		addOpt(0, T("splash_col_default"))
		addOpt(1, T("smf_title")+" (A-Z)")
		addOpt(2, T("smf_title")+" (Z-A)")
		addOpt(3, T("smf_artist")+" (A-Z)")
		addOpt(4, T("smf_album")+" (A-Z)")
	case vAlbums:
		addOpt(0, T("smf_artist")+" (A-Z)")
		addOpt(1, T("smf_album")+" (A-Z)")
		addOpt(2, T("smf_album")+" (Z-A)")
		addOpt(3, fmt.Sprintf("%s ↓", T("smf_track")))
	case vArtists:
		addOpt(0, T("smf_artist")+" (A-Z)")
		addOpt(1, T("smf_artist")+" (Z-A)")
		addOpt(2, fmt.Sprintf("%s ↓", T("smf_track")))
		addOpt(3, fmt.Sprintf("%s ↓", T("smf_album")))
	case vPlaylists:
		addOpt(0, T("smf_title")+" (A-Z)")
		addOpt(1, T("smf_title")+" (Z-A)")
		addOpt(2, fmt.Sprintf("%s ↓", T("smf_track")))
	case vAlbum:
		addOpt(0, T("smf_track"))
		addOpt(1, T("smf_title")+" (A-Z)")
		addOpt(2, T("smf_title")+" (Z-A)")
	case vPlaylist:
		addOpt(0, T("splash_col_default"))
		addOpt(1, T("smf_title")+" (A-Z)")
		addOpt(2, T("smf_artist")+" (A-Z)")
	case vFolder:
		addOpt(0, "A-Z")
		addOpt(1, "Z-A")
	}

	a.menu = &menu{title: T("sm_sort", ""), items: items}
}

// spotifySortLabel devuelve el texto de ordenación de pistas de Spotify (Liked o Open Playlist).
func (a *App) spotifySortLabel() string {
	s := &a.spot
	val := a.sortState.spotLiked
	if s.openPl != nil {
		val = a.sortState.spotTracks
	}
	switch val {
	case 1:
		return T("smf_title") + " (A-Z)"
	case 2:
		return T("smf_title") + " (Z-A)"
	case 3:
		return T("smf_artist") + " (A-Z)"
	case 4:
		return fmt.Sprintf("%s ↓", T("smf_duration"))
	default:
		return T("splash_col_default")
	}
}

// spotifyPlSortLabel devuelve el texto de ordenación de listas/álbumes de Spotify.
func (a *App) spotifyPlSortLabel() string {
	s := &a.spot
	if s.tab == spAlbums {
		switch a.sortState.spotAlbums {
		case 1:
			return T("smf_album") + " (A-Z)"
		case 2:
			return T("smf_artist") + " (A-Z)"
		case 3:
			return fmt.Sprintf("%s ↓", T("smf_track"))
		default:
			return T("splash_col_default")
		}
	}
	switch a.sortState.spotPls {
	case 1:
		return T("smf_title") + " (A-Z)"
	case 2:
		return T("smf_title") + " (Z-A)"
	case 3:
		return fmt.Sprintf("%s ↓", T("smf_track"))
	default:
		return T("splash_col_default")
	}
}

// applySpotifySort aplica la ordenación elegida en la vista actual de Spotify.
func (a *App) applySpotifySort() {
	s := &a.spot
	if s.openPl != nil {
		switch a.sortState.spotTracks {
		case 1:
			sort.SliceStable(s.openPl.Tracks, func(i, j int) bool {
				return strings.ToLower(s.openPl.Tracks[i].Title) < strings.ToLower(s.openPl.Tracks[j].Title)
			})
		case 2:
			sort.SliceStable(s.openPl.Tracks, func(i, j int) bool {
				if !strings.EqualFold(s.openPl.Tracks[i].Artist, s.openPl.Tracks[j].Artist) {
					return strings.ToLower(s.openPl.Tracks[i].Artist) < strings.ToLower(s.openPl.Tracks[j].Artist)
				}
				return strings.ToLower(s.openPl.Tracks[i].Title) < strings.ToLower(s.openPl.Tracks[j].Title)
			})
		case 3:
			sort.SliceStable(s.openPl.Tracks, func(i, j int) bool {
				return s.openPl.Tracks[i].Duration > s.openPl.Tracks[j].Duration
			})
		}
		s.openPlSel = 0
		return
	}

	switch s.tab {
	case spLiked:
		switch a.sortState.spotLiked {
		case 1:
			sort.SliceStable(s.liked, func(i, j int) bool {
				return strings.ToLower(s.liked[i].Title) < strings.ToLower(s.liked[j].Title)
			})
		case 2:
			sort.SliceStable(s.liked, func(i, j int) bool {
				return strings.ToLower(s.liked[i].Title) > strings.ToLower(s.liked[j].Title)
			})
		case 3:
			sort.SliceStable(s.liked, func(i, j int) bool {
				if !strings.EqualFold(s.liked[i].Artist, s.liked[j].Artist) {
					return strings.ToLower(s.liked[i].Artist) < strings.ToLower(s.liked[j].Artist)
				}
				return strings.ToLower(s.liked[i].Title) < strings.ToLower(s.liked[j].Title)
			})
		case 4:
			sort.SliceStable(s.liked, func(i, j int) bool {
				return s.liked[i].Duration > s.liked[j].Duration
			})
		}
		s.sel[spLiked] = 0
	case spPlaylists:
		switch a.sortState.spotPls {
		case 1:
			sort.SliceStable(s.playlists, func(i, j int) bool {
				return strings.ToLower(s.playlists[i].Name) < strings.ToLower(s.playlists[j].Name)
			})
		case 2:
			sort.SliceStable(s.playlists, func(i, j int) bool {
				return strings.ToLower(s.playlists[i].Name) > strings.ToLower(s.playlists[j].Name)
			})
		case 3:
			sort.SliceStable(s.playlists, func(i, j int) bool {
				return s.playlists[i].TrackCount > s.playlists[j].TrackCount
			})
		}
		s.sel[spPlaylists] = 0
	case spAlbums:
		switch a.sortState.spotAlbums {
		case 1:
			sort.SliceStable(s.albums, func(i, j int) bool {
				return strings.ToLower(s.albums[i].Name) < strings.ToLower(s.albums[j].Name)
			})
		case 2:
			sort.SliceStable(s.albums, func(i, j int) bool {
				return strings.ToLower(s.albums[i].Name) < strings.ToLower(s.albums[j].Name)
			})
		case 3:
			sort.SliceStable(s.albums, func(i, j int) bool {
				return s.albums[i].TrackCount > s.albums[j].TrackCount
			})
		}
		s.sel[spAlbums] = 0
	}
}

// openSpotifySortMenu despliega el menú de ordenación para pistas de Spotify.
func (a *App) openSpotifySortMenu() {
	s := &a.spot
	var items []menuItem
	addOpt := func(val int, label string) {
		items = append(items, menuItem{
			label: label,
			icon:  "list",
			do: func() {
				if s.openPl != nil {
					a.sortState.spotTracks = val
				} else {
					a.sortState.spotLiked = val
				}
				a.applySpotifySort()
				a.flash(T("sm_sort", a.spotifySortLabel()))
			},
		})
	}
	addOpt(0, T("splash_col_default"))
	addOpt(1, T("smf_title")+" (A-Z)")
	addOpt(2, T("smf_title")+" (Z-A)")
	addOpt(3, T("smf_artist")+" (A-Z)")
	addOpt(4, fmt.Sprintf("%s ↓", T("smf_duration")))

	a.menu = &menu{title: T("sm_sort", ""), items: items}
}

// openSpotifyPlSortMenu despliega el menú de ordenación para listas/álbumes de Spotify.
func (a *App) openSpotifyPlSortMenu() {
	s := &a.spot
	var items []menuItem
	if s.tab == spAlbums {
		addOpt := func(val int, label string) {
			items = append(items, menuItem{
				label: label,
				icon:  "list",
				do: func() {
					a.sortState.spotAlbums = val
					a.applySpotifySort()
					a.flash(T("sm_sort", a.spotifyPlSortLabel()))
				},
			})
		}
		addOpt(0, T("splash_col_default"))
		addOpt(1, T("smf_album")+" (A-Z)")
		addOpt(2, T("smf_artist")+" (A-Z)")
		addOpt(3, fmt.Sprintf("%s ↓", T("smf_track")))
	} else {
		addOpt := func(val int, label string) {
			items = append(items, menuItem{
				label: label,
				icon:  "list",
				do: func() {
					a.sortState.spotPls = val
					a.applySpotifySort()
					a.flash(T("sm_sort", a.spotifyPlSortLabel()))
				},
			})
		}
		addOpt(0, T("splash_col_default"))
		addOpt(1, T("smf_title")+" (A-Z)")
		addOpt(2, T("smf_title")+" (Z-A)")
		addOpt(3, fmt.Sprintf("%s ↓", T("smf_track")))
	}

	a.menu = &menu{title: T("sm_sort", ""), items: items}
}

// Suppress unused imports
var _ = slices.Index[[]int]
