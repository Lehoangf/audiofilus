package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const releaseVersion = "v0.22.1"
const pkgPath = "github.com/juanvillacortac/audiofilus/internal/version.Version=" + releaseVersion

func main() {
	fmt.Printf("=== BUILDING AND PACKAGING AUDIOFILUS RELEASE %s ===\n", releaseVersion)

	outDir := "out"
	_ = os.RemoveAll(outDir)
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal("mkdir out", err)
	}
	_ = os.MkdirAll("dist", 0o755)

	// 1. Build Linux ARM64 (Handhelds & PortMaster)
	fmt.Println("[1/6] Building Linux ARM64 binary...")
	cmdARM64 := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X "+pkgPath, "-o", "dist/audiofilus.aarch64", "./cmd/audiofilus")
	cmdARM64.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=arm64")
	if out, err := cmdARM64.CombinedOutput(); err != nil {
		fatal("build linux arm64: "+string(out), err)
	}

	// Also copy to standalone in out/
	copyFile("dist/audiofilus.aarch64", filepath.Join(outDir, "audiofilus.aarch64"), 0o755)
	// Update dist mirror
	copyFile("dist/audiofilus.aarch64", "dist/audiofilus-spotify.aarch64", 0o755)
	copyFile("dist/audiofilus.aarch64", "dist/muxapp_spotify/audiofilus-spotify/audiofilus.aarch64", 0o755)

	// 2. Build Linux AMD64 (Desktop)
	fmt.Println("[2/6] Building Linux AMD64 binary...")
	cmdAMD64 := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X "+pkgPath, "-o", "dist/audiofilus-amd64", "./cmd/audiofilus")
	cmdAMD64.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	if out, err := cmdAMD64.CombinedOutput(); err != nil {
		fatal("build linux amd64: "+string(out), err)
	}

	// 3. Build Windows AMD64
	fmt.Println("[3/5] Building Windows AMD64 binary...")
	cmdWin := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -H=windowsgui -X "+pkgPath, "-o", "dist/audiofilus.exe", "./cmd/audiofilus")
	cmdWin.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64")
	if out, err := cmdWin.CombinedOutput(); err != nil {
		fatal("build windows: "+string(out), err)
	}

	// 4. Package formats
	fmt.Println("[4/5] Packaging release archives...")

	// 4a. PortMaster: out/audiofilus-portmaster.zip
	packPortMaster(filepath.Join(outDir, "audiofilus-portmaster.zip"))

	// 4b. muOS: out/audiofilus.muxapp
	packMuxApp(filepath.Join(outDir, "audiofilus.muxapp"))

	// 4c. Windows: out/audiofilus-windows-amd64.zip
	packWindowsZip(filepath.Join(outDir, "audiofilus-windows-amd64.zip"))

	// 4d. Linux Desktop tarballs
	packLinuxTarGz("dist/audiofilus-amd64", filepath.Join(outDir, "audiofilus-linux-amd64.tar.gz"), "audiofilus-linux-amd64")
	packLinuxTarGz("dist/audiofilus.aarch64", filepath.Join(outDir, "audiofilus-linux-arm64.tar.gz"), "audiofilus-linux-arm64")

	// 5. Generate SHA256SUMS
	fmt.Println("[5/5] Computing SHA256 checksums...")
	writeSHA256Sums(outDir)

	fmt.Println("\n=== ALL PACKAGES SUCCESSFULLY CREATED IN out/ ===")
	listDirectory(outDir)
}

func packPortMaster(outZip string) {
	fmt.Println("  -> Packaging PortMaster:", outZip)
	zipFile, err := os.Create(outZip)
	if err != nil {
		fatal("create portmaster zip", err)
	}
	defer zipFile.Close()
	w := zip.NewWriter(zipFile)
	defer w.Close()

	addFileToZip(w, "portmaster/Audiofilus.sh", "Audiofilus.sh", 0o755)
	if _, err := os.Stat("portmaster/gameinfo.xml"); err == nil {
		addFileToZip(w, "portmaster/gameinfo.xml", "gameinfo.xml", 0o644)
	}

	srcBase := "dist/muxapp_spotify/audiofilus-spotify"
	_ = filepath.Walk(srcBase, func(p string, fi os.FileInfo, err error) error {
		if err != nil || p == srcBase {
			return err
		}
		rel, err := filepath.Rel(srcBase, p)
		if err != nil {
			return err
		}
		targetPath := filepath.ToSlash(filepath.Join("audiofilus", rel))
		if fi.IsDir() {
			return nil
		}
		mode := os.FileMode(0o644)
		name := filepath.Base(p)
		if name == "audiofilus.aarch64" || strings.HasPrefix(name, "yt-dlp") || strings.HasSuffix(name, ".so") || strings.Contains(name, ".so.") {
			mode = 0o755
		}
		addFileToZip(w, p, targetPath, mode)
		return nil
	})
}

func packMuxApp(outZip string) {
	fmt.Println("  -> Packaging muOS .muxapp:", outZip)
	srcDir := "dist/muxapp_spotify"
	outFile, err := os.Create(outZip)
	if err != nil {
		fatal("create muxapp", err)
	}
	defer outFile.Close()
	w := zip.NewWriter(outFile)
	defer w.Close()

	_ = filepath.Walk(srcDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil || p == srcDir {
			return err
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		zipPath := filepath.ToSlash(rel)
		if fi.IsDir() {
			return nil
		}
		mode := os.FileMode(0o644)
		name := filepath.Base(p)
		if name == "mux_launch.sh" || name == "audiofilus.aarch64" || name == "audiofilus" || strings.HasPrefix(name, "yt-dlp") || strings.Contains(name, ".so") {
			mode = 0o755
		}
		addFileToZip(w, p, zipPath, mode)
		return nil
	})
}

func packWindowsZip(outZip string) {
	fmt.Println("  -> Packaging Windows zip:", outZip)
	f, err := os.Create(outZip)
	if err != nil {
		fatal("create windows zip", err)
	}
	defer f.Close()
	w := zip.NewWriter(f)
	defer w.Close()

	addFileToZip(w, "dist/audiofilus.exe", "audiofilus.exe", 0o755)
	if _, err := os.Stat("audiofilus.json"); err == nil {
		addFileToZip(w, "audiofilus.json", "audiofilus.json", 0o644)
	}
	if _, err := os.Stat("LICENSE"); err == nil {
		addFileToZip(w, "LICENSE", "LICENSE.txt", 0o644)
	}
	if _, err := os.Stat("README.md"); err == nil {
		addFileToZip(w, "README.md", "README.md", 0o644)
	}
}

func packLinuxTarGz(binPath, outTarGz, rootPrefix string) {
	fmt.Println("  -> Packaging Linux tar.gz:", outTarGz)
	f, err := os.Create(outTarGz)
	if err != nil {
		fatal("create linux tar.gz", err)
	}
	defer f.Close()
	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	addFileToTar(tw, binPath, rootPrefix+"/audiofilus", 0o755)
	if _, err := os.Stat("LICENSE"); err == nil {
		addFileToTar(tw, "LICENSE", rootPrefix+"/LICENSE", 0o644)
	}
	if _, err := os.Stat("README.md"); err == nil {
		addFileToTar(tw, "README.md", rootPrefix+"/README.md", 0o644)
	}
}

func writeSHA256Sums(outDir string) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		fatal("read outDir for checksums", err)
	}
	var lines []string
	for _, e := range entries {
		if e.IsDir() || e.Name() == "SHA256SUMS" {
			continue
		}
		fp := filepath.Join(outDir, e.Name())
		data, err := os.ReadFile(fp)
		if err != nil {
			fatal("read for hash: "+fp, err)
		}
		h := sha256.Sum256(data)
		lines = append(lines, fmt.Sprintf("%s  %s", hex.EncodeToString(h[:]), e.Name()))
	}
	sort.Strings(lines)
	sumFile := filepath.Join(outDir, "SHA256SUMS")
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(sumFile, []byte(content), 0o644); err != nil {
		fatal("write SHA256SUMS", err)
	}
}

func addFileToZip(w *zip.Writer, srcPath, zipPath string, mode os.FileMode) {
	fi, err := os.Stat(srcPath)
	if err != nil {
		fatal("stat "+srcPath, err)
	}
	header, err := zip.FileInfoHeader(fi)
	if err != nil {
		fatal("header "+srcPath, err)
	}
	header.Name = zipPath
	header.Method = zip.Deflate
	header.SetMode(mode)

	writer, err := w.CreateHeader(header)
	if err != nil {
		fatal("create header in zip "+zipPath, err)
	}
	f, err := os.Open(srcPath)
	if err != nil {
		fatal("open "+srcPath, err)
	}
	defer f.Close()
	if _, err := io.Copy(writer, f); err != nil {
		fatal("copy to zip "+zipPath, err)
	}
}

func addFileToTar(tw *tar.Writer, srcPath, tarPath string, mode int64) {
	fi, err := os.Stat(srcPath)
	if err != nil {
		fatal("stat "+srcPath, err)
	}
	hdr := &tar.Header{
		Name:    tarPath,
		Mode:    mode,
		Size:    fi.Size(),
		ModTime: fi.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		fatal("tar header "+tarPath, err)
	}
	f, err := os.Open(srcPath)
	if err != nil {
		fatal("open "+srcPath, err)
	}
	defer f.Close()
	if _, err := io.Copy(tw, f); err != nil {
		fatal("tar copy "+tarPath, err)
	}
}

func copyFile(src, dst string, mode os.FileMode) {
	in, err := os.Open(src)
	if err != nil {
		fatal("open src "+src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		fatal("open dst "+dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		fatal("copy "+src+" to "+dst, err)
	}
}

func listDirectory(dir string) {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		info, _ := e.Info()
		fmt.Printf("  • %-32s (%10d bytes)\n", e.Name(), info.Size())
	}
}

func fatal(msg string, err error) {
	fmt.Printf("[FATAL] %s: %v\n", msg, err)
	os.Exit(1)
}
