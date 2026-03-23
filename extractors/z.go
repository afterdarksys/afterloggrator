package extractors

import (
	"compress/lzw"
	"io"
	"os"
)

func init() {
	register(".z", func(file *os.File, filename string) (io.ReadCloser, error) {
		return lzw.NewReader(file, lzw.LSB, 8), nil
	})
	
	// Support lowercase .z fallback occasionally used for LZW
	register(".Z", func(file *os.File, filename string) (io.ReadCloser, error) {
		return lzw.NewReader(file, lzw.LSB, 8), nil
	})
}
