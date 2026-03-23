package extractors

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

type ReaderFactory func(file *os.File, filename string) (io.ReadCloser, error)

var registry = make(map[string]ReaderFactory)

func register(ext string, factory ReaderFactory) {
	registry[strings.ToLower(ext)] = factory
}

// GetReader checks the extension and routes to the appropriate decompression routine
func GetReader(file *os.File, filename string) (io.ReadCloser, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if factory, ok := registry[ext]; ok {
		return factory(file, filename)
	}
	return file, nil // Fallback to raw uncompressed bytes if unsupported
}
