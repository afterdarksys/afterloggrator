package extractors

import (
	"archive/tar"
	"io"
	"os"
)

type tarStreamReader struct {
	tr     *tar.Reader
	active bool
	last   byte
	had    bool
	closer io.Closer
}

func (t *tarStreamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if !t.active {
			h, err := t.tr.Next()
			if err != nil {
				return 0, err
			}
			if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
				continue
			}
			t.active = true
			t.had = false
		}
		n, err := t.tr.Read(p)
		if n > 0 {
			t.had = true
			t.last = p[n-1]
			if err != nil && err != io.EOF {
				return n, err
			}
			return n, nil
		}
		if err != io.EOF {
			return n, err
		}
		t.active = false
		if t.had && t.last != '\n' {
			p[0] = '\n'
			return 1, nil
		}
	}
}
func (t *tarStreamReader) Close() error {
	if t.closer != nil {
		return t.closer.Close()
	}
	return nil
}
func init() {
	register(".tar", func(file *os.File, _ string) (io.ReadCloser, error) {
		return &tarStreamReader{tr: tar.NewReader(file)}, nil
	})
}
