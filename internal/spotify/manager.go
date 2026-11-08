package spotify

import (
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/juanvillacortac/audiofilus/internal/decode"
)

// IsSpotify determina si una URI o ruta corresponde a una pista de Spotify (spotify://track/...).
func IsSpotify(uri string) bool {
	return strings.HasPrefix(uri, "spotify:track:") || strings.HasPrefix(uri, "spotify://track/")
}

// ExtractTrackID obtiene el identificador de pista de una URI de Spotify.
func ExtractTrackID(uri string) string {
	if strings.HasPrefix(uri, "spotify://track/") {
		return strings.TrimPrefix(uri, "spotify://track/")
	}
	if strings.HasPrefix(uri, "spotify:track:") {
		return strings.TrimPrefix(uri, "spotify:track:")
	}
	return uri
}

// Manager coordina la autenticación, catálogo y reproducción de Spotify en el servicio.
type Manager struct {
	dataPath string
	Auth     *AuthManager
	Catalog  *Catalog
	Resolver *StreamResolver
	client   *http.Client
	mu       sync.Mutex
	tracks   map[string]Track
}

// NewManager inicializa un nuevo gestor de Spotify para el servicio daemon.
func NewManager(dataPath string) *Manager {
	auth := NewAuthManager(dataPath, "")
	transport := NewTransport(3.0, 5, http.DefaultTransport)
	client := &http.Client{Transport: transport}
	cacheDir := filepath.Join(dataPath, "cache", "spotify")

	cat := NewCatalog(cacheDir, auth, client)
	res := NewStreamResolver(cacheDir, auth, client)

	return &Manager{
		dataPath: dataPath,
		Auth:     auth,
		Catalog:  cat,
		Resolver: res,
		client:   client,
		tracks:   make(map[string]Track),
	}
}

// Source resuelve el archivo de audio local para una pista de Spotify y lo abre como decode.Source.
func (m *Manager) Source(uri string) (decode.Source, bool, error) {
	trackID := ExtractTrackID(uri)
	m.mu.Lock()
	tr, ok := m.tracks[trackID]
	if !ok {
		tr = Track{ID: trackID, Title: "Spotify Track", Artist: "Spotify"}
	}
	m.mu.Unlock()

	localPath, ready, err := m.Resolver.Resolve(tr)
	if err != nil || !ready {
		return nil, false, err
	}

	src, err := decode.Open(localPath)
	return src, err == nil, err
}

// RegisterTrack asocia metadatos a un trackID para su resolución posterior.
func (m *Manager) RegisterTrack(t Track) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracks[t.ID] = t
}

// Track devuelve los metadatos registrados para una URI de Spotify.
func (m *Manager) Track(uri string) (Track, bool) {
	trackID := ExtractTrackID(uri)
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tracks[trackID]
	return t, ok
}
