package extractors

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveMemberBoundaries(t *testing.T) {
	for _, ext := range []string{".tar", ".zip", ".tar.gz", ".tgz"} {
		var b bytes.Buffer
		if ext == ".zip" {
			w := zip.NewWriter(&b)
			for _, name := range []string{"one", "two"} {
				f, _ := w.Create(name)
				f.Write([]byte(name))
			}
			w.Close()
		} else {
			var dest io.Writer = &b
			var gz *gzip.Writer
			if ext != ".tar" {
				gz = gzip.NewWriter(&b)
				dest = gz
			}
			w := tar.NewWriter(dest)
			w.WriteHeader(&tar.Header{Name: "directory/", Typeflag: tar.TypeDir, Mode: 0755})
			for _, name := range []string{"one", "two"} {
				w.WriteHeader(&tar.Header{Name: name, Size: int64(len(name)), Mode: 0600})
				w.Write([]byte(name))
			}
			w.Close()
			if gz != nil {
				gz.Close()
			}
		}
		path := filepath.Join(t.TempDir(), "logs"+ext)
		os.WriteFile(path, b.Bytes(), 0600)
		f, _ := os.Open(path)
		r, err := GetReader(f, path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		f.Close()
		if err != nil || string(data) != "one\ntwo\n" {
			t.Fatalf("%s: %q %v", ext, data, err)
		}
	}
}
func TestUnsupportedZ(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "*.Z")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = GetReader(f, f.Name()); err == nil {
		t.Fatal("incorrect LZW decoder accepted")
	}
}
func TestShellQuote(t *testing.T) {
	if shellQuote("a'; touch /tmp/b") != "'a'\"'\"'; touch /tmp/b'" {
		t.Fatal("incorrect shell quoting")
	}
}
