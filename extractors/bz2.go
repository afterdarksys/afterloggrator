package extractors

import (
	"compress/bzip2"
	"io"
	"os"
)

type nopCloser struct {
	io.Reader
}

func (nopCloser) Close() error { return nil }

func init() {
	register(".bz2", func(file *os.File, filename string) (io.ReadCloser, error) {
		// bzip2.NewReader does not return an io.ReadCloser natively, so we wrap it
		return nopCloser{bzip2.NewReader(file)}, nil
	})
}
