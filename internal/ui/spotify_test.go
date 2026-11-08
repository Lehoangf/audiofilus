package ui

import (
	"testing"
)

func TestSpotifyLocalization(t *testing.T) {
	for _, lang := range []string{"es", "en", "pt", "fr", "it", "de", "pl", "tr", "ru", "id", "ja", "zh", "ko"} {
		SetLanguage(lang)
		if T("spotify") == "" || T("spotify_liked") == "" {
			t.Errorf("missing spotify translations for lang %s", lang)
		}
	}
}

func TestSpotifyScreenEnum(t *testing.T) {
	if Spotify.String() != "spotify" {
		t.Errorf("expected Spotify.String() == 'spotify', got %q", Spotify.String())
	}
}

func TestSpotifyStateTabSwitching(t *testing.T) {
	s := &spotifyState{}
	if s.tab != spLiked {
		t.Errorf("expected initial tab spLiked (0), got %d", s.tab)
	}
	s.tab = spPlaylists
	if s.tab != spPlaylists {
		t.Errorf("expected spPlaylists, got %d", s.tab)
	}
	s.tab = spAlbums
	if s.tab != spAlbums {
		t.Errorf("expected spAlbums, got %d", s.tab)
	}
	s.tab = spSearch
	if s.tab != spSearch {
		t.Errorf("expected spSearch, got %d", s.tab)
	}
}
