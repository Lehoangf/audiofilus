// Package update checks for new releases on GitHub and installs them over the
// running application: downloads the appropriate package for this platform
// (PortMaster port, muOS app, Linux tar.gz, or Windows zip), verifies its
// SHA256 against SHA256SUMS, and replaces files atomically.
// Configuration (audiofilus.json, data/) is never touched.
package update

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Repo is the GitHub repository releases are fetched from.
const Repo = "Lehoangf/audiofilus"

// apiBase is the GitHub API base URL (overridden in tests).
var apiBase = "https://api.github.com"

// client is the HTTP client for release queries and downloads.
var client = &http.Client{Timeout: 10 * time.Minute}

// Release represents a published GitHub release.
type Release struct {
	Tag    string            // e.g. "v0.9.0"
	Notes  string            // Release notes in Markdown
	URL    string            // Web page URL
	Assets map[string]string // asset name -> download URL
}

// Latest returns the latest published release.
func Latest(ctx context.Context) (*Release, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", apiBase+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	var r struct {
		Tag    string `json:"tag_name"`
		Body   string `json:"body"`
		URL    string `json:"html_url"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	rel := &Release{Tag: r.Tag, Notes: r.Body, URL: r.URL, Assets: map[string]string{}}
	for _, a := range r.Assets {
		rel.Assets[a.Name] = a.URL
	}
	return rel, nil
}

// parse parses "v1.2.3" (or "1.2.3"); ok is false if not a valid semver triplet.
func parse(v string) (n [3]int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return n, false
	}
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil {
			return n, false
		}
		n[i] = x
	}
	return n, true
}

// Newer returns true if tag is strictly newer than current version.
// If current is non-versioned (e.g. "dev"), it returns false.
func Newer(current, tag string) bool {
	c, ok1 := parse(current)
	t, ok2 := parse(tag)
	if !ok1 || !ok2 {
		return false
	}
	for i := range c {
		if t[i] != c[i] {
			return t[i] > c[i]
		}
	}
	return false
}

// Kind identifies the installation packaging format.
type Kind int

const (
	None       Kind = iota // Unknown / manual copy: self-updates disabled
	PortMaster             // PortMaster port: audiofilus-portmaster.zip
	MuOS                   // muOS application package: audiofilus.muxapp
	Linux                  // Linux desktop tarball: audiofilus-linux-<arch>.tar.gz
	Windows                // Windows zip: audiofilus-windows-amd64.zip
)

// Detect inspects executable directory and name to determine installation kind.
func Detect(exe string) Kind {
	dir, base := filepath.Dir(exe), filepath.Base(exe)
	exists := func(name string) bool { _, err := os.Stat(filepath.Join(dir, name)); return err == nil }
	switch {
	case exists("mux_launch.sh"):
		return MuOS
	case exists("port.json"):
		return PortMaster
	case runtime.GOOS == "windows" && strings.EqualFold(base, "audiofilus.exe"):
		return Windows
	case runtime.GOOS == "linux" && base == "audiofilus":
		return Linux
	}
	return None
}

// Asset returns the release asset filename for kind k.
func Asset(k Kind) string {
	switch k {
	case PortMaster:
		return "audiofilus-portmaster.zip"
	case MuOS:
		return "audiofilus.muxapp"
	case Linux:
		return "audiofilus-linux-" + runtime.GOARCH + ".tar.gz"
	case Windows:
		return "audiofilus-windows-amd64.zip"
	}
	return ""
}

// prefix returns the archive root directory to strip when extracting.
func prefix(k Kind) string {
	switch k {
	case PortMaster, MuOS:
		return "audiofilus/"
	case Linux:
		return "audiofilus-linux-" + runtime.GOARCH + "/"
	}
	return ""
}

// Progress receives download progress bytes (total may be -1 if unknown).
type Progress func(done, total int64)

// Install downloads and installs release rel into the executable's directory.
func Install(ctx context.Context, rel *Release, exe string, progress Progress) error {
	k := Detect(exe)
	name := Asset(k)
	if name == "" {
		return errors.New("this installation can't update itself")
	}
	data, err := verified(ctx, rel, name, progress)
	if err != nil {
		return err
	}
	files, err := extract(k, data)
	if err != nil {
		return err
	}
	return place(k, filepath.Dir(exe), files)
}

// APK is the Android application package filename.
const APK = "audiofilus.apk"

// Fetch downloads release asset name to dest, verifying SHA256.
func Fetch(ctx context.Context, rel *Release, name, dest string, progress Progress) error {
	data, err := verified(ctx, rel, name, progress)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}

// verified downloads asset name and verifies its checksum against SHA256SUMS.
func verified(ctx context.Context, rel *Release, name string, progress Progress) ([]byte, error) {
	url, ok := rel.Assets[name]
	if !ok {
		return nil, fmt.Errorf("%s: not in release %s", name, rel.Tag)
	}
	sumsURL, ok := rel.Assets["SHA256SUMS"]
	if !ok {
		return nil, fmt.Errorf("SHA256SUMS: not in release %s", rel.Tag)
	}
	sums, err := download(ctx, sumsURL, nil)
	if err != nil {
		return nil, err
	}
	want, err := sumFor(sums, name)
	if err != nil {
		return nil, err
	}
	data, err := download(ctx, url, progress)
	if err != nil {
		return nil, err
	}
	if got := sha256.Sum256(data); hex.EncodeToString(got[:]) != want {
		return nil, fmt.Errorf("%s: checksum mismatch", name)
	}
	return data, nil
}

// download downloads an entire URL into memory with progress updates.
func download(ctx context.Context, url string, progress Progress) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", path.Base(url), resp.Status)
	}
	var buf bytes.Buffer
	chunk := make([]byte, 64<<10)
	for {
		n, err := resp.Body.Read(chunk)
		buf.Write(chunk[:n])
		if progress != nil {
			progress(int64(buf.Len()), resp.ContentLength)
		}
		if err == io.EOF {
			return buf.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// sumFor retrieves the SHA256 hash for name from a SHA256SUMS formatted content.
func sumFor(sums []byte, name string) (string, error) {
	sc := bufio.NewScanner(bytes.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("%s: no checksum in SHA256SUMS", name)
}

// file represents an extracted package file entry.
type file struct {
	path string // Relative to executable directory ("../Audiofilus.sh" for port launcher)
	data []byte
	exec bool
}

// extract extracts installation files from archive data.
func extract(k Kind, data []byte) ([]file, error) {
	var out []file
	add := func(name string, mode os.FileMode, r io.Reader) error {
		rel, ok := strings.CutPrefix(name, prefix(k))
		switch {
		case k == PortMaster && (name == "Audiofilus.sh" || name == "gameinfo.xml"):
			rel = "../" + name // Port launcher placed next to port directory
		case !ok, rel == "", strings.HasSuffix(rel, "/"):
			return nil
		case strings.Contains(rel, ".."), path.IsAbs(rel):
			return nil // Prevent directory traversal
		case rel == "audiofilus.json", strings.HasPrefix(rel, "data/"):
			return nil // Never overwrite user configuration or database
		}
		b, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		out = append(out, file{rel, b, mode&0o111 != 0 || isExec(rel)})
		return nil
	}
	if k == Linux {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		tr := tar.NewReader(zr)
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if h.Typeflag == tar.TypeReg {
				if err := add(h.Name, os.FileMode(h.Mode), tr); err != nil {
					return nil, err
				}
			}
		}
	} else {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			err = add(f.Name, f.Mode(), rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("the package is empty")
	}
	return out, nil
}

// isExec checks if a file requires executable permissions.
func isExec(rel string) bool {
	base := path.Base(rel)
	return strings.HasSuffix(base, ".aarch64") || strings.HasSuffix(base, ".x86_64") ||
		strings.HasSuffix(base, ".sh") || base == "audiofilus" || strings.HasSuffix(base, ".exe")
}

// place writes extracted files to dir atomically via .new files.
// On Windows, in-use executables are renamed to .old first.
func place(k Kind, dir string, files []file) error {
	for _, f := range files {
		dst := filepath.Join(dir, filepath.FromSlash(f.path))
		if strings.HasPrefix(f.path, "../") {
			if _, err := os.Stat(dst); err != nil {
				continue
			}
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if f.exec {
			mode = 0o755
		}
		tmp := dst + ".new"
		if err := os.WriteFile(tmp, f.data, mode); err != nil {
			return err
		}
		if k == Windows {
			if _, err := os.Stat(dst); err == nil {
				_ = os.Remove(dst + ".old")
				if err := os.Rename(dst, dst+".old"); err != nil {
					os.Remove(tmp)
					return err
				}
			}
		}
		if err := os.Rename(tmp, dst); err != nil {
			os.Remove(tmp)
			return err
		}
	}
	return nil
}

// Cleanup removes leftover .old files on Windows.
func Cleanup(exe string) {
	if runtime.GOOS != "windows" {
		return
	}
	olds, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "*.old"))
	for _, o := range olds {
		_ = os.Remove(o)
	}
}
