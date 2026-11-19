package lyrics

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HasLyrics comprueba si una canción ya dispone de letras locales (.lrc, .txt,
// etiquetas embebidas o archivo .lrc junto al audio).
func HasLyrics(audioPath string) bool {
	if !IsLocal(audioPath) {
		return false
	}
	// Comprueba si ya existe un archivo .lrc directo.
	lrc := LRCPath(audioPath)
	if st, err := os.Stat(lrc); err == nil && !st.IsDir() && st.Size() > 0 {
		return true
	}
	// Comprueba letras locales (.lrc, .txt en carpeta o subcarpeta lyrics, y etiquetas).
	for _, o := range Local(audioPath) {
		if !o.Guess && !o.Lyrics.Empty() {
			return true
		}
	}
	return false
}

// LRCPath devuelve la ruta del archivo .lrc correspondiente junto al archivo de audio.
func LRCPath(audioPath string) string {
	ext := filepath.Ext(audioPath)
	return strings.TrimSuffix(audioPath, ext) + ".lrc"
}

// SaveLRC guarda el contenido de la letra en el archivo .lrc junto al audio.
func SaveLRC(audioPath, content string) error {
	dest := LRCPath(audioPath)
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("empty lyrics content")
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	// Escribe a un archivo temporal en el mismo directorio y renombra atómicamente.
	dir := filepath.Dir(dest)
	tmp, err := os.CreateTemp(dir, ".lrc-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}

// FetchAndSaveLRC busca en LRCLIB la letra para la canción especificada y la guarda
// como archivo .lrc junto al audio. Devuelve true si encontró y guardó la letra.
func FetchAndSaveLRC(ctx context.Context, web *LRCLIB, store *Store, q Query, audioPath string) (bool, error) {
	if HasLyrics(audioPath) {
		return false, nil
	}
	if web == nil {
		return false, nil
	}
	found, err := web.Search(ctx, q)
	if err != nil {
		return false, err
	}
	best, ok := Best(found, q)
	if !ok || best.Instrumental {
		if store != nil && err == nil && ctx.Err() == nil {
			_ = store.Update(audioPath, func(c *Choice) { c.Missed = time.Now() })
		}
		return false, nil
	}

	rawText := strings.TrimSpace(best.Synced)
	if rawText == "" {
		rawText = strings.TrimSpace(best.Plain)
	}
	if rawText == "" {
		return false, nil
	}

	if err := SaveLRC(audioPath, rawText); err != nil {
		return false, err
	}

	if store != nil {
		_ = store.saveFound(best)
		_ = store.Update(audioPath, func(c *Choice) {
			c.Source = fmt.Sprintf("lrclib:%d", best.ID)
			c.Auto = true
			c.Missed = time.Time{}
		})
	}
	return true, nil
}
