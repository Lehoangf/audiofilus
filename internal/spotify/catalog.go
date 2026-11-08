package spotify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Track representa una pista musical de Spotify con metadatos esenciales.
type Track struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Artist   string        `json:"artist"`
	Album    string        `json:"album"`
	ISRC     string        `json:"isrc,omitempty"`
	Duration time.Duration `json:"duration"`
	CoverURL string        `json:"cover_url,omitempty"`
}

// Playlist representa una lista de reproducción con su identificador de instantánea (snapshot).
type Playlist struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	SnapshotID string  `json:"snapshot_id"`
	Tracks     []Track `json:"tracks"`
}

// catalogData es la estructura persistida en disco para la sincronización diferencial.
type catalogData struct {
	Playlists   []Playlist `json:"playlists"`
	LikedTracks []Track    `json:"liked_tracks"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Catalog gestiona la sincronización y la caché delta de listas y canciones de Spotify.
type Catalog struct {
	cacheDir string
	auth     *AuthManager
	client   *http.Client
	apiBase  string
	mu       sync.Mutex
}

// NewCatalog crea un nuevo gestor de catálogo local.
func NewCatalog(cacheDir string, auth *AuthManager, client *http.Client) *Catalog {
	if client == nil {
		client = http.DefaultClient
	}
	return &Catalog{
		cacheDir: cacheDir,
		auth:     auth,
		client:   client,
		apiBase:  "https://api.spotify.com/v1",
	}
}

func (c *Catalog) cacheFile() string {
	return filepath.Join(c.cacheDir, "catalog.json")
}

func (c *Catalog) loadCached() catalogData {
	b, err := os.ReadFile(c.cacheFile())
	if err != nil {
		return catalogData{}
	}
	var cd catalogData
	_ = json.Unmarshal(b, &cd)
	return cd
}

func (c *Catalog) saveCached(cd catalogData) error {
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return err
	}
	cd.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cd, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.cacheFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, c.cacheFile())
}

func (c *Catalog) newAuthorizedRequest(method, urlStr string) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return nil, err
	}
	if c.auth != nil {
		token, err := c.auth.ProactiveToken()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req, nil
}

// SyncPlaylists sincroniza las listas de reproducción aplicando delta sync por snapshot_id.
func (c *Catalog) SyncPlaylists() ([]Playlist, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached := c.loadCached()
	cachedMap := make(map[string]Playlist)
	for _, pl := range cached.Playlists {
		cachedMap[pl.ID] = pl
	}

	req, err := c.newAuthorizedRequest(http.MethodGet, c.apiBase+"/me/playlists?limit=50")
	if err != nil {
		return cached.Playlists, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return cached.Playlists, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cached.Playlists, fmt.Errorf("fetch playlists status: %d", resp.StatusCode)
	}

	var res struct {
		Items []struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			SnapshotID string `json:"snapshot_id"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return cached.Playlists, err
	}

	var updated []Playlist
	for _, item := range res.Items {
		cPl, exists := cachedMap[item.ID]
		// Delta Sync: si el snapshot_id no ha cambiado y tenemos pistas, reutilizarlas sin llamar a la API.
		if exists && cPl.SnapshotID == item.SnapshotID && len(cPl.Tracks) > 0 {
			cPl.Name = item.Name
			updated = append(updated, cPl)
			continue
		}

		tracks, err := c.fetchPlaylistTracks(item.ID)
		if err != nil {
			// Si falla la red para esta lista en particular, usar la versión en caché si existe.
			if exists {
				updated = append(updated, cPl)
			}
			continue
		}

		updated = append(updated, Playlist{
			ID:         item.ID,
			Name:       item.Name,
			SnapshotID: item.SnapshotID,
			Tracks:     tracks,
		})
	}

	cached.Playlists = updated
	_ = c.saveCached(cached)
	return updated, nil
}

func (c *Catalog) fetchPlaylistTracks(playlistID string) ([]Track, error) {
	req, err := c.newAuthorizedRequest(http.MethodGet, fmt.Sprintf("%s/playlists/%s/tracks?limit=100", c.apiBase, playlistID))
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch playlist tracks status: %d", resp.StatusCode)
	}

	var res struct {
		Items []struct {
			Track struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DurationMS  int    `json:"duration_ms"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"track"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}

	var tracks []Track
	for _, it := range res.Items {
		t := it.Track
		if t.ID == "" {
			continue
		}
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		var cover string
		if len(t.Album.Images) > 0 {
			cover = t.Album.Images[0].URL
		}
		tracks = append(tracks, Track{
			ID:       t.ID,
			Title:    t.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    t.Album.Name,
			ISRC:     t.ExternalIDs.ISRC,
			Duration: time.Duration(t.DurationMS) * time.Millisecond,
			CoverURL: cover,
		})
	}
	return tracks, nil
}

// LikedSongs obtiene las canciones favoritas ("Canciones que te gustan") del usuario.
func (c *Catalog) LikedSongs() ([]Track, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cached := c.loadCached()

	req, err := c.newAuthorizedRequest(http.MethodGet, c.apiBase+"/me/tracks?limit=50")
	if err != nil {
		return cached.LikedTracks, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return cached.LikedTracks, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return cached.LikedTracks, fmt.Errorf("fetch liked tracks status: %d", resp.StatusCode)
	}

	var res struct {
		Items []struct {
			Track struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DurationMS  int    `json:"duration_ms"`
				ExternalIDs struct {
					ISRC string `json:"isrc"`
				} `json:"external_ids"`
				Artists []struct {
					Name string `json:"name"`
				} `json:"artists"`
				Album struct {
					Name   string `json:"name"`
					Images []struct {
						URL string `json:"url"`
					} `json:"images"`
				} `json:"album"`
			} `json:"track"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return cached.LikedTracks, err
	}

	var tracks []Track
	for _, it := range res.Items {
		t := it.Track
		if t.ID == "" {
			continue
		}
		var artists []string
		for _, a := range t.Artists {
			artists = append(artists, a.Name)
		}
		var cover string
		if len(t.Album.Images) > 0 {
			cover = t.Album.Images[0].URL
		}
		tracks = append(tracks, Track{
			ID:       t.ID,
			Title:    t.Name,
			Artist:   strings.Join(artists, ", "),
			Album:    t.Album.Name,
			ISRC:     t.ExternalIDs.ISRC,
			Duration: time.Duration(t.DurationMS) * time.Millisecond,
			CoverURL: cover,
		})
	}

	cached.LikedTracks = tracks
	_ = c.saveCached(cached)
	return tracks, nil
}
