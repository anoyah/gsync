package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"testing"
)

func TestArchives(t *testing.T) {
	for _, windows := range []bool{false, true} {
		name := "gsync"
		if windows {
			name += ".exe"
		}
		data, err := pack(name, []byte("binary"), []byte("readme"), windows)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]string{}
		if windows {
			z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range z.File {
				r, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				b, err := io.ReadAll(r)
				r.Close()
				if err != nil {
					t.Fatal(err)
				}
				files[file.Name] = string(b)
			}
		} else {
			gz, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			defer gz.Close()
			tr := tar.NewReader(gz)
			for {
				h, err := tr.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				b, err := io.ReadAll(tr)
				if err != nil {
					t.Fatal(err)
				}
				files[h.Name] = string(b)
				if h.Name == name && h.Mode != 0755 {
					t.Fatal("binary not executable")
				}
			}
		}
		if len(files) != 2 || files[name] != "binary" || files["README.md"] != "readme" {
			t.Fatalf("invalid archive: %v", files)
		}
	}
}
