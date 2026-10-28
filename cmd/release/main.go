// Command release builds local release archives. It never publishes or uploads.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type target struct{ os, arch string }

var targets = []target{{"darwin", "amd64"}, {"darwin", "arm64"}, {"linux", "amd64"}, {"linux", "arm64"}, {"windows", "amd64"}, {"windows", "arm64"}}
var validVersion = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

func main() {
	version := flag.String("version", "", "required release version, e.g. v0.1.0")
	dir := flag.String("out", "dist", "output parent directory")
	flag.Parse()
	if !validVersion.MatchString(*version) || flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/release --version v0.1.0 [--out dist]")
		os.Exit(2)
	}
	if err := build(*version, *dir, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func build(version, parent string, log io.Writer) error {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		return err
	}
	license, err := os.ReadFile("LICENSE")
	if err != nil {
		return err
	}
	notices, err := os.ReadFile("THIRD_PARTY_NOTICES.md")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	out := filepath.Join(parent, version)
	if _, err := os.Lstat(out); err == nil {
		return fmt.Errorf("output already exists: %s", out)
	} else if !os.IsNotExist(err) {
		return err
	}
	// Assemble in a fresh directory; failed builds never appear as a complete release.
	work, err := os.MkdirTemp(parent, ".release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	var sums strings.Builder
	for _, t := range targets {
		name := "gsync"
		if t.os == "windows" {
			name += ".exe"
		}
		binary := filepath.Join(work, name)
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -X main.version="+version, "-o", binary, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+t.os, "GOARCH="+t.arch)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("build %s/%s: %w\n%s", t.os, t.arch, err, output)
		}
		data, err := os.ReadFile(binary)
		if err != nil {
			return err
		}
		ext := ".tar.gz"
		if t.os == "windows" {
			ext = ".zip"
		}
		filename := "gsync_" + version + "_" + t.os + "_" + t.arch + ext
		archive, err := pack(name, data, readme, license, notices, t.os == "windows")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(work, filename), archive, 0644); err != nil {
			return err
		}
		if err := os.Remove(binary); err != nil {
			return err
		}
		fmt.Fprintf(&sums, "%x  %s\n", sha256.Sum256(archive), filename)
		fmt.Fprintln(log, filename)
	}
	if err := os.WriteFile(filepath.Join(work, "SHA256SUMS"), []byte(sums.String()), 0644); err != nil {
		return err
	}
	if err := os.Rename(work, out); err != nil {
		return err
	}
	fmt.Fprintln(log, "Release archives:", out)
	return nil
}

func pack(binary string, data, readme, license, notices []byte, windows bool) ([]byte, error) {
	files := []struct {
		name string
		data []byte
		mode int64
	}{{binary, data, 0755}, {"README.md", readme, 0644}, {"LICENSE", license, 0644}, {"THIRD_PARTY_NOTICES.md", notices, 0644}}
	var buf bytes.Buffer
	// Fixed metadata makes packaging the same build reproducible.
	stamp := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	if windows {
		z := zip.NewWriter(&buf)
		for _, file := range files {
			h := &zip.FileHeader{Name: file.name, Method: zip.Deflate, Modified: stamp}
			h.SetMode(os.FileMode(file.mode))
			w, err := z.CreateHeader(h)
			if err != nil {
				return nil, err
			}
			if _, err := w.Write(file.data); err != nil {
				return nil, err
			}
		}
		if err := z.Close(); err != nil {
			return nil, err
		}
	} else {
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		for _, file := range files {
			if err := tw.WriteHeader(&tar.Header{Name: file.name, Mode: file.mode, Size: int64(len(file.data)), ModTime: stamp, Typeflag: tar.TypeReg}); err != nil {
				return nil, err
			}
			if _, err := tw.Write(file.data); err != nil {
				return nil, err
			}
		}
		if err := tw.Close(); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
