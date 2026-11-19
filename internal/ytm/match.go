package ytm

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// AudioExts son las extensiones de audio buscadas en las carpetas de música,
// en el orden más probable para descargas y copias locales.
var AudioExts = []string{".m4a", ".mp3", ".flac", ".ogg", ".opus", ".wav", ".aac", ".wma"}

var audioExtSet = func() map[string]bool {
	m := make(map[string]bool, len(AudioExts)+4)
	for _, ext := range AudioExts {
		m[ext] = true
	}
	m[".alac"] = true
	m[".aiff"] = true
	m[".aif"] = true
	m[".mp4"] = true
	return m
}()

// IsAudioExt indica si ext (con o sin punto inicial) es una extensión de audio local.
func IsAudioExt(ext string) bool {
	if ext == "" {
		return false
	}
	ext = strings.ToLower(ext)
	if ext[0] != '.' {
		ext = "." + ext
	}
	return audioExtSet[ext]
}

var foldReplacer = strings.NewReplacer(
	// Símbolos de SafeName de vuelta a su forma normal antes de limpiar signos.
	"⧸", "/", "⧹", "\\", "：", ":", "？", "?", "＂", "\"", "＊", "*", "｜", "|", "＜", "<", "＞", ">",
	// Vietnamita, español, portugués, francés, alemán y otras lenguas latinas.
	"đ", "d", "ð", "d", "þ", "th",
	"á", "a", "à", "a", "ả", "a", "ã", "a", "ạ", "a",
	"ă", "a", "ắ", "a", "ằ", "a", "ẳ", "a", "ẵ", "a", "ặ", "a",
	"â", "a", "ấ", "a", "ầ", "a", "ẩ", "a", "ẫ", "a", "ậ", "a",
	"ä", "a", "å", "a", "ā", "a", "ą", "a",
	"é", "e", "è", "e", "ẻ", "e", "ẽ", "e", "ẹ", "e",
	"ê", "e", "ế", "e", "ề", "e", "ể", "e", "ễ", "e", "ệ", "e",
	"ë", "e", "ē", "e", "ę", "e", "ě", "e",
	"í", "i", "ì", "i", "ỉ", "i", "ĩ", "i", "ị", "i",
	"î", "i", "ï", "i", "ī", "i", "ı", "i",
	"ó", "o", "ò", "o", "ỏ", "o", "õ", "o", "ọ", "o",
	"ô", "o", "ố", "o", "ồ", "o", "ổ", "o", "ỗ", "o", "ộ", "o",
	"ơ", "o", "ớ", "o", "ờ", "o", "ở", "o", "ỡ", "o", "ợ", "o",
	"ö", "o", "ø", "o", "ō", "o",
	"ú", "u", "ù", "u", "ủ", "u", "ũ", "u", "ụ", "u",
	"ư", "u", "ứ", "u", "ừ", "u", "ử", "u", "ữ", "u", "ự", "u",
	"û", "u", "ü", "u", "ū", "u", "ů", "u",
	"ý", "y", "ỳ", "y", "ỷ", "y", "ỹ", "y", "ỵ", "y", "ÿ", "y",
	"ñ", "n", "ń", "n", "ň", "n",
	"ç", "c", "ć", "c", "č", "c",
	"ł", "l", "ś", "s", "š", "s", "ş", "s", "ß", "ss",
	"ź", "z", "ž", "z", "ż", "z",
	"ğ", "g", "ř", "r", "ď", "d", "ť", "t",
	"æ", "ae", "œ", "oe",
)

// Fold pasa s a minúsculas sin tildes ni signos diacríticos.
func Fold(s string) string {
	if s == "" {
		return ""
	}
	s = foldReplacer.Replace(strings.ToLower(s))
	// Quita marcas combinadas por si algún archivo viene en NFD (macOS/exFAT).
	hasMn := false
	for _, r := range s {
		if unicode.Is(unicode.Mn, r) {
			hasMn = true
			break
		}
	}
	if !hasMn {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return foldReplacer.Replace(b.String())
}

// CleanWords deja solo letras y dígitos en minúsculas y sin tildes, separados por un espacio.
func CleanWords(s string) string {
	return strings.Join(strings.FieldsFunc(Fold(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

// FirstArtist devuelve el primer artista de una lista separada por comas ("A, B" → "A").
func FirstArtist(s string) string {
	if idx := strings.Index(s, ","); idx > 0 {
		return strings.TrimSpace(s[:idx])
	}
	return strings.TrimSpace(s)
}

// ArtistCandidates devuelve los nombres de carpeta posibles para artist (completo y primer artista).
func ArtistCandidates(artist string) []string {
	full := strings.TrimSpace(artist)
	if full == "" {
		return nil
	}
	first := FirstArtist(full)
	if first == "" || first == full {
		return []string{full}
	}
	return []string{full, first}
}

// ArtistMatches comprueba si el artista de Spotify coincide con el artista o álbum-artista local.
func ArtistMatches(spArtist, candArtist, candAlbumArtist string) bool {
	spClean := CleanWords(spArtist)
	spFirst := CleanWords(FirstArtist(spArtist))
	if spClean == "" && spFirst == "" {
		return false
	}
	var spParts []string
	for _, p := range strings.Split(spArtist, ",") {
		if pw := CleanWords(p); pw != "" {
			spParts = append(spParts, pw)
		}
	}
	for _, cand := range []string{candArtist, candAlbumArtist} {
		if cand == "" {
			continue
		}
		cClean := CleanWords(cand)
		cFirst := CleanWords(FirstArtist(cand))
		if cClean == "" {
			continue
		}
		if cClean == spClean || (spFirst != "" && (cFirst == spFirst || cClean == spFirst)) || (cFirst != "" && cFirst == spClean) {
			return true
		}
		for _, cp := range strings.Split(cand, ",") {
			cpClean := CleanWords(cp)
			if cpClean == "" {
				continue
			}
			for _, sp := range spParts {
				if cpClean == sp {
					return true
				}
			}
		}
	}
	return false
}

// StripTrackPrefix quita el número de pista inicial de un nombre de archivo ("14. Canción" → "Canción"),
// exigiendo un separador explícito para no romper títulos como "7 Rings" o "24 Hours".
func StripTrackPrefix(stem string) string {
	s := strings.TrimSpace(stem)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' && i < 3 {
		i++
	}
	if i == 0 || i >= len(s) {
		return stem
	}
	j := i
	for j < len(s) && s[j] == ' ' {
		j++
	}
	if j >= len(s) {
		return stem
	}
	switch s[j] {
	case '.', '-', '_', ')':
		for j < len(s) && (s[j] == '.' || s[j] == '-' || s[j] == '_' || s[j] == ')' || s[j] == ' ') {
			j++
		}
		if j < len(s) {
			return s[j:]
		}
	}
	return stem
}

var suffixTriggerWords = map[string]bool{
	"feat": true, "ft": true, "featuring": true, "with": true,
	"remaster": true, "remastered": true,
	"remix": true, "live": true, "acoustic": true, "instrumental": true,
	"karaoke": true, "acapella": true, "sped": true, "slowed": true,
	"reverb": true, "nightcore": true, "8d": true, "cover": true,
	"vip": true, "dub": true, "extended": true, "edit": true,
	"demo": true, "remake": true, "piano": true, "orchestral": true,
	"unplugged": true, "session": true, "mix": true, "radio": true,
	"club": true, "stripped": true, "version": true, "ver": true,
	"explicit": true, "clean": true, "single": true, "album": true,
	"bonus": true, "audio": true, "official": true, "lyric": true,
	"lyrics": true, "video": true, "mono": true, "stereo": true,
	"deluxe": true, "original": true,
}

var distinguishingVersionWords = map[string]bool{
	"remix": true, "live": true, "acoustic": true, "instrumental": true,
	"karaoke": true, "acapella": true, "sped": true, "slowed": true,
	"reverb": true, "nightcore": true, "8d": true, "cover": true,
	"vip": true, "dub": true, "extended": true, "edit": true,
	"demo": true, "remake": true, "piano": true, "orchestral": true,
	"unplugged": true, "session": true, "mix": true, "radio": true,
	"club": true, "stripped": true,
}

func splitTitleAndVersion(title string) (base string, versionTags string) {
	var mainBuf strings.Builder
	var suffixWords []string

	// 1. Extrae lo que va entre paréntesis o corchetes.
	depth := 0
	var seg strings.Builder
	for _, r := range title {
		switch r {
		case '(', '[':
			if depth == 0 {
				seg.Reset()
			}
			depth++
		case ')', ']':
			if depth > 0 {
				depth--
				if depth == 0 {
					suffixWords = append(suffixWords, strings.Fields(CleanWords(seg.String()))...)
				}
			}
		default:
			if depth == 0 {
				mainBuf.WriteRune(r)
			} else {
				seg.WriteRune(r)
			}
		}
	}
	main := strings.TrimSpace(mainBuf.String())
	if main == "" {
		// Título entero entre paréntesis: no vaciar el título base.
		main = strings.TrimSpace(title)
		suffixWords = nil
	}

	// 2. Separa un sufijo tras " - " si lleva palabras de versión, remaster o feat.
	for off := 0; ; {
		idx := strings.Index(main[off:], " - ")
		if idx < 0 {
			break
		}
		cut := off + idx
		left := strings.TrimSpace(main[:cut])
		right := strings.TrimSpace(main[cut+3:])
		if left != "" && hasAnyWord(right, suffixTriggerWords) {
			main = left
			suffixWords = append(suffixWords, strings.Fields(CleanWords(right))...)
			break
		}
		off = cut + 3
	}

	// 3. Conserva solo palabras distintivas de versión (ignora feat./remaster/etc.).
	var tags []string
	seen := make(map[string]bool)
	for _, w := range suffixWords {
		if distinguishingVersionWords[w] && !seen[w] {
			seen[w] = true
			tags = append(tags, w)
		}
	}
	sort.Strings(tags)
	return CleanWords(main), strings.Join(tags, ",")
}

func hasAnyWord(s string, set map[string]bool) bool {
	for _, w := range strings.Fields(CleanWords(s)) {
		if set[w] {
			return true
		}
	}
	return false
}

// BaseTitleAndVersion devuelve el título base limpio y su firma de versión (p. ej. "live,remix").
func BaseTitleAndVersion(title string) (string, string) {
	return splitTitleAndVersion(title)
}

// TitleMatches compara un título candidato (de etiqueta o archivo) contra el título buscado,
// permitiendo diferencias de tildes, feat./remaster, pero distinguiendo remix/live/acústica.
func TitleMatches(candTitle, wantTitle string, allowTruncatedStem bool) bool {
	if candTitle == "" || wantTitle == "" {
		return false
	}
	cClean := CleanWords(candTitle)
	wClean := CleanWords(wantTitle)
	if cClean != "" && cClean == wClean {
		return true
	}

	if allowTruncatedStem {
		trimmed := strings.TrimSpace(candTitle)
		if strings.HasSuffix(trimmed, "…") || strings.HasSuffix(trimmed, "...") {
			stemPrefix := strings.TrimRight(trimmed, ".… ")
			pClean := CleanWords(stemPrefix)
			if len([]rune(pClean)) >= 8 && strings.HasPrefix(wClean, pClean) {
				_, cVer := splitTitleAndVersion(stemPrefix)
				_, wVer := splitTitleAndVersion(wantTitle)
				if cVer == wVer {
					return true
				}
			}
		}
	}

	cBase, cVer := splitTitleAndVersion(candTitle)
	wBase, wVer := splitTitleAndVersion(wantTitle)
	if cBase == "" || wBase == "" {
		return false
	}
	return cBase == wBase && cVer == wVer
}

// FileTitleMatches comprueba si el nombre de archivo name corresponde al título wantTitle.
func FileTitleMatches(name, wantTitle string) bool {
	ext := filepath.Ext(name)
	if !IsAudioExt(ext) {
		return false
	}
	stem := strings.TrimSuffix(name, ext)
	if stem == SafeName(wantTitle) {
		return true
	}
	stripped := StripTrackPrefix(stem)
	return TitleMatches(stripped, wantTitle, true) || TitleMatches(stem, wantTitle, true)
}

func isNonEmptyFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

// FindLocalAudio busca una canción existente dentro de dirs por ruta convencional,
// carpeta de artista (con cualquier álbum/subcarpeta o suelta) y prefijos de pista.
func FindLocalAudio(dirs []string, artist, album, title string) string {
	if title == "" || len(dirs) == 0 {
		return ""
	}
	artists := ArtistCandidates(artist)
	if len(artists) == 0 {
		return ""
	}
	alb := album
	if alb == "" {
		alb = "Singles"
	}
	safeTitle := SafeName(title)
	safeAlb := SafeName(alb)

	// 1. Comprobación directa de ruta convencional: Artista/Álbum/Título.ext y Artista/Título.ext.
	for _, dir := range dirs {
		for _, art := range artists {
			artDir := filepath.Join(dir, SafeName(art))
			for _, ext := range AudioExts {
				p := filepath.Join(artDir, safeAlb, safeTitle+ext)
				if isNonEmptyFile(p) {
					return p
				}
			}
			for _, ext := range AudioExts {
				p := filepath.Join(artDir, safeTitle+ext)
				if isNonEmptyFile(p) {
					return p
				}
			}
		}
	}

	// 2. Explora solo la carpeta del artista (otras ediciones del álbum o números de pista).
	for _, dir := range dirs {
		var matchedArtDirs []string
		seenArtDirs := make(map[string]bool)
		for _, art := range artists {
			artDir := filepath.Join(dir, SafeName(art))
			if fi, err := os.Stat(artDir); err == nil && fi.IsDir() {
				if !seenArtDirs[artDir] {
					seenArtDirs[artDir] = true
					matchedArtDirs = append(matchedArtDirs, artDir)
				}
			}
		}
		if len(matchedArtDirs) == 0 {
			if topEntries, err := os.ReadDir(dir); err == nil {
				for _, te := range topEntries {
					if !te.IsDir() {
						continue
					}
					if ArtistMatches(artist, te.Name(), "") {
						p := filepath.Join(dir, te.Name())
						if !seenArtDirs[p] {
							seenArtDirs[p] = true
							matchedArtDirs = append(matchedArtDirs, p)
						}
					}
				}
			}
		}
		for _, artDir := range matchedArtDirs {
			entries, err := os.ReadDir(artDir)
			if err != nil {
				continue
			}
			var subdirs []string
			for _, entry := range entries {
				if entry.IsDir() {
					subDir := filepath.Join(artDir, entry.Name())
					for _, ext := range AudioExts {
						p := filepath.Join(subDir, safeTitle+ext)
						if isNonEmptyFile(p) {
							return p
						}
					}
					subdirs = append(subdirs, subDir)
					continue
				}
				if FileTitleMatches(entry.Name(), title) {
					p := filepath.Join(artDir, entry.Name())
					if isNonEmptyFile(p) {
						return p
					}
				}
			}
			for _, subDir := range subdirs {
				subEntries, err := os.ReadDir(subDir)
				if err != nil {
					continue
				}
				for _, se := range subEntries {
					if se.IsDir() {
						continue
					}
					if FileTitleMatches(se.Name(), title) {
						p := filepath.Join(subDir, se.Name())
						if isNonEmptyFile(p) {
							return p
						}
					}
				}
			}
		}
	}
	return ""
}
