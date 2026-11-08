package daemon

import (
	"testing"
)

func TestSpotifyTrackURLRecognition(t *testing.T) {
	uri := "spotify://track/4cOdK2wGLETKBW3PvgPWqT"
	if !isSpotifyURI(uri) {
		t.Errorf("expected %s to be recognized as spotify URI", uri)
	}
	invalid := "https://example.com/song.mp3"
	if isSpotifyURI(invalid) {
		t.Errorf("expected %s NOT to be recognized as spotify URI", invalid)
	}
}
