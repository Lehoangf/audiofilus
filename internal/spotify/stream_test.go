package spotify

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecryptSpotifyOgg(t *testing.T) {
	key := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	expectedData := []byte("OggS_valid_vorbis_stream_content")
	// Spotify container: 167 bytes header + AES-128-CTR encrypted payload
	header := make([]byte, 167)
	block, _ := aes.NewCipher(key[:])
	iv := make([]byte, 16) // Counter starts at 0
	stream := cipher.NewCTR(block, iv)
	encryptedPayload := make([]byte, len(expectedData))
	stream.XORKeyStream(encryptedPayload, expectedData)

	fullData := append(header, encryptedPayload...)
	decrypted, err := DecryptSpotifyOgg(bytes.NewReader(fullData), key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	out, _ := io.ReadAll(decrypted)
	if !bytes.Equal(out, expectedData) {
		t.Errorf("got %q, want %q", out, expectedData)
	}
}

func TestOfflineCacheRetrieval(t *testing.T) {
	tempDir := t.TempDir()
	sr := NewStreamResolver(tempDir, nil, nil)

	tr := Track{
		ID:       "offline123",
		Title:    "Offline Song",
		Artist:   "Cached Artist",
		Duration: 3 * time.Minute,
	}

	// Simular archivo ya descargado en la caché
	expectedPath := filepath.Join(tempDir, "songs", tr.ID+".audio")
	_ = os.MkdirAll(filepath.Dir(expectedPath), 0o755)
	_ = os.WriteFile(expectedPath, []byte("fake_audio_stream"), 0o644)

	path, ready, err := sr.Resolve(tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready || path != expectedPath {
		t.Errorf("expected cached path %s and ready=true, got ready=%v, path=%s", expectedPath, ready, path)
	}
}

func TestFallbackMatchFailure(t *testing.T) {
	tempDir := t.TempDir()
	sr := NewStreamResolver(tempDir, nil, nil)
	// Mock fallback solver que no encuentra coincidencia
	sr.ytmFinder = func(query string) (string, error) {
		return "", os.ErrNotExist
	}

	tr := Track{
		ID:       "notfound404",
		Title:    "Obscure Track",
		Artist:   "Unknown",
		Duration: 2 * time.Minute,
	}

	_, ready, err := sr.Resolve(tr)
	if err == nil {
		t.Errorf("expected error when fallback matching fails, got nil")
	}
	if ready {
		t.Errorf("expected ready=false on fallback match failure")
	}
}
