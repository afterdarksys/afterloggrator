package extractors

import (
	"compress/gzip"
	"io"
	"os"
)

func init() {
	register(".gz", func(file *os.File, filename string) (io.ReadCloser, error) {
		return gzip.NewReader(file)
	})
}
