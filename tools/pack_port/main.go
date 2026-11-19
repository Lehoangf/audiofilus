package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	outZip := "dist/audiofilus-portmaster.zip"
	_ = os.MkdirAll("dist", 0o755)

	zipFile, err := os.Create(outZip)
	if err != nil {
		fmt.Printf("Error creating zip: %v\n", err)
		os.Exit(1)
	}
	defer zipFile.Close()

	w := zip.NewWriter(zipFile)
	defer w.Close()

	fmt.Printf("Packaging PortMaster port into %s with Unix permissions...\n", outZip)

	// Add root PortMaster files: Audiofilus.sh, gameinfo.xml
	addFileToZip(w, "portmaster/Audiofilus.sh", "Audiofilus.sh", 0o755)
	if _, err := os.Stat("portmaster/gameinfo.xml"); err == nil {
		addFileToZip(w, "portmaster/gameinfo.xml", "gameinfo.xml", 0o644)
	}

	// Add audiofilus directory contents from dist/muxapp_spotify/audiofilus-spotify
	srcBase := "dist/muxapp_spotify/audiofilus-spotify"
	err = filepath.Walk(srcBase, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcBase, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Skip mux_launch.sh and glyph in portmaster folder
		if rel == "mux_launch.sh" || strings.HasPrefix(rel, "glyph") {
			return nil
		}

		destPath := "audiofilus/" + filepath.ToSlash(rel)
		if fi.IsDir() {
			header := &zip.FileHeader{
				Name:   destPath + "/",
				Method: zip.Deflate,
			}
			header.SetMode(0o755 | os.ModeDir)
			_, err = w.CreateHeader(header)
			return err
		}

		mode := os.FileMode(0o644)
		name := fi.Name()
		if strings.HasSuffix(name, ".aarch64") || strings.HasSuffix(name, ".so") || strings.Contains(name, ".so.") ||
			strings.HasPrefix(name, "yt-dlp") || strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}

		addFileToZip(w, p, destPath, mode)
		return nil
	})
	if err != nil {
		fmt.Printf("Error walking srcBase: %v\n", err)
		os.Exit(1)
	}

	// Add PortMaster specific items: port.json, cover.png, screenshot.png
	for _, f := range []string{"port.json", "cover.png", "screenshot.png"} {
		src := filepath.Join("portmaster/audiofilus", f)
		if _, err := os.Stat(src); err == nil {
			addFileToZip(w, src, "audiofilus/"+f, 0o644)
		}
	}

	fmt.Printf("Successfully created %s\n", outZip)
}

func addFileToZip(w *zip.Writer, srcPath, zipPath string, mode os.FileMode) {
	fi, err := os.Stat(srcPath)
	if err != nil {
		return
	}
	f, err := os.Open(srcPath)
	if err != nil {
		return
	}
	defer f.Close()

	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		return
	}
	header.Name = zipPath
	header.Method = zip.Deflate
	header.SetMode(mode)

	writer, err := w.CreateHeader(header)
	if err != nil {
		return
	}
	_, _ = io.Copy(writer, f)
	fmt.Printf("  Added: %s (mode: %v)\n", zipPath, mode)
}
