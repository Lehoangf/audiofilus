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
	srcDir := "dist/muxapp_spotify"
	outZip := "dist/audiofilus-spotify.muxapp"

	fmt.Printf("Packaging %s into %s with Unix permissions...\n", srcDir, outZip)

	outFile, err := os.Create(outZip)
	if err != nil {
		fmt.Printf("Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer outFile.Close()

	w := zip.NewWriter(outFile)
	defer w.Close()

	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		zipPath := filepath.ToSlash(relPath)

		var mode os.FileMode
		if info.IsDir() {
			mode = os.ModeDir | 0o755
		} else {
			name := filepath.Base(path)
			if name == "mux_launch.sh" || name == "audiofilus.aarch64" || name == "audiofilus" || strings.HasPrefix(name, "yt-dlp") || strings.HasSuffix(name, ".sh") || strings.Contains(name, ".so") {
				mode = 0o755
			} else {
				mode = 0o644
			}
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = zipPath
		if info.IsDir() {
			header.Name += "/"
		}
		header.Method = zip.Deflate
		header.SetMode(mode)

		writer, err := w.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			defer f.Close()

			if _, err := io.Copy(writer, f); err != nil {
				return err
			}
		}

		fmt.Printf("  Added: %s (mode: %v)\n", header.Name, mode)
		return nil
	})

	if err != nil {
		fmt.Printf("Packaging failed: %v\n", err)
		os.Exit(1)
	}

	if err := w.Close(); err != nil {
		fmt.Printf("Error closing zip writer: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully created", outZip)
}
