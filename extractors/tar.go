package extractors

import (
	"archive/tar"
	"io"
	"os"
)

type tarStreamReader struct {
	tr *tar.Reader
}

func (t *tarStreamReader) Read(p []byte) (int, error) {
	n, err := t.tr.Read(p)
	if n > 0 {
		return n, nil
	}
	if err == io.EOF {
		_, nextErr := t.tr.Next()
		if nextErr != nil {
			return 0, nextErr
		}
		return t.Read(p)
	}
	return n, err
}

func (t *tarStreamReader) Close() error {
	return nil
}

func init() {
	register(".tar", func(file *os.File, filename string) (io.ReadCloser, error) {
		tr := tar.NewReader(file)
		return &tarStreamReader{tr: tr}, nil
	})
}
