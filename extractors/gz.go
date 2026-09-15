package extractors

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"strings"
)

func gzipReader(file *os.File, name string) (io.ReadCloser, error) {
	r, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") {
		return &tarStreamReader{tr: tar.NewReader(r), closer: r}, nil
	}
	return r, nil
}
func init() { register(".gz", gzipReader); register(".tgz", gzipReader) }
