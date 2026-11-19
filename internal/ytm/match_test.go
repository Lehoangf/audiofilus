package ytm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindLocalAudioVariants(t *testing.T) {
	dir := t.TempDir()

	// 1. Multi-artist folder written by SaveM4A ("Wren Evans, itsnk/<album>/<title>.m4a")
	f1 := filepath.Join(dir, SafeName("Wren Evans, itsnk"), SafeName("LOI CHOI: The Neo Pop Punk"), SafeName("Tò Te Tí")+".m4a")
	if err := os.MkdirAll(filepath.Dir(f1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f1, []byte("m4a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindLocalAudio([]string{dir}, "Wren Evans, itsnk", "LOI CHOI: The Neo Pop Punk", "Tò Te Tí"); got != f1 {
		t.Errorf("multi-artist full folder = %q, want %q", got, f1)
	}

	// 2. Alternate album edition ("(side A)") + truncated filename ("…")
	f2 := filepath.Join(dir, "GREY D", SafeName("ÁNH SÁNG • MÀN ĐÊM (side A)"), "yêu em như….m4a")
	if err := os.MkdirAll(filepath.Dir(f2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2, []byte("m4a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindLocalAudio([]string{dir}, "GREY D", "ÁNH SÁNG • MÀN ĐÊM", "yêu em như thế nào thì em sẽ ở lại?"); got != f2 {
		t.Errorf("alternate album + truncated stem = %q, want %q", got, f2)
	}

	// 3. Flat artist folder + track number prefix + FLAC extension ("MCK/14. Ai Mới Là Kẻ Xấu Xa.flac")
	f3 := filepath.Join(dir, "MCK", "14. Ai Mới Là Kẻ Xấu Xa.flac")
	if err := os.MkdirAll(filepath.Dir(f3), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f3, []byte("flac"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindLocalAudio([]string{dir}, "MCK, JustaTee", "99%", "Ai Mới Là Kẻ Xấu Xa"); got != f3 {
		t.Errorf("flat flac + track prefix = %q, want %q", got, f3)
	}

	// 4. First artist folder with "(ft. ...)" suffix vs bare Spotify title
	f4 := filepath.Join(dir, "Obito", "Đánh Đổi", "Đánh Đổi (ft. RPT MCK).m4a")
	if err := os.MkdirAll(filepath.Dir(f4), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f4, []byte("m4a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindLocalAudio([]string{dir}, "Obito, RPT MCK, Shiki", "Đánh Đổi", "Đánh Đổi"); got != f4 {
		t.Errorf("featured suffix match = %q, want %q", got, f4)
	}
}

func TestVersionAndPrefixSafety(t *testing.T) {
	// Different versions must NOT match each other.
	if TitleMatches("Hype", "Hype (Remix)", false) {
		t.Error("plain title should not match Remix")
	}
	if TitleMatches("Hotel California", "Hotel California - Live on MTV, 1994", false) {
		t.Error("studio title should not match Live")
	}
	if TitleMatches("Song", "Song (Acoustic)", false) {
		t.Error("plain title should not match Acoustic")
	}

	// Compatible versions and remasters SHOULD match.
	if !TitleMatches("Song (Acoustic)", "Song - Acoustic Version", false) {
		t.Error("expected matching Acoustic versions")
	}
	if !TitleMatches("Yesterday", "Yesterday - Remastered 2009", false) {
		t.Error("expected studio title to match Remastered")
	}
	if !TitleMatches("Song (Official Audio)", "Song (feat. Guest)", false) {
		t.Error("expected feat. and Official Audio to match")
	}

	// Numeric song titles without explicit separator must not lose leading numbers.
	if got := StripTrackPrefix("7 Rings"); got != "7 Rings" {
		t.Errorf("StripTrackPrefix(%q) = %q, want unchanged", "7 Rings", got)
	}
	if got := StripTrackPrefix("24 Hours"); got != "24 Hours" {
		t.Errorf("StripTrackPrefix(%q) = %q, want unchanged", "24 Hours", got)
	}
	if got := StripTrackPrefix("14. Ai Mới Là Kẻ Xấu Xa"); got != "Ai Mới Là Kẻ Xấu Xa" {
		t.Errorf("StripTrackPrefix(numbered) = %q, want stripped", got)
	}
}

func TestArtistMatches(t *testing.T) {
	if !ArtistMatches("Wren Evans, itsnk", "Wren Evans", "") {
		t.Error("expected multi-artist to match primary artist")
	}
	if !ArtistMatches("Obito", "Obito, RPT MCK", "Obito") {
		t.Error("expected primary artist to match multi-artist local tag")
	}
	if !ArtistMatches("RPT MCK, Obito", "Obito", "") {
		t.Error("expected reordered multi-artist list to match collaborator")
	}
	if ArtistMatches("Simon & Garfunkel", "Simon", "") {
		t.Error("band name with '&' must not match solo unrelated artist")
	}
}

func TestFindLocalAudioCaseAndDiacriticFolder(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "Son Tung M-TP", "Chung Ta Cua Hien Tai.mp3")
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, []byte("mp3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindLocalAudio([]string{dir}, "Sơn Tùng M-TP", "", "Chúng Ta Của Hiện Tại"); got != f {
		t.Errorf("diacritic folder fallback = %q, want %q", got, f)
	}
}
