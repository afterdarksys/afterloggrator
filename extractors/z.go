package extractors

import (
	"fmt"
	"io"
	"os"
)

func init() {
	register(".z", func(_ *os.File, _ string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("Unix compress .Z is unsupported; decompress it with uncompress first")
	})
}
