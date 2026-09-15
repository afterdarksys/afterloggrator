package extractors

import (
	"archive/zip"
	"io"
	"os"
)

type zipStreamReader struct {
	zr     *zip.Reader
	index  int
	currRc io.ReadCloser
	had    bool
	last   byte
}

func (z *zipStreamReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if z.currRc == nil {
			if z.index >= len(z.zr.File) {
				return 0, io.EOF
			}
			f := z.zr.File[z.index]
			z.index++
			if f.FileInfo().IsDir() {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return 0, err
			}
			z.currRc = rc
			z.had = false
		}
		n, err := z.currRc.Read(p)
		if n > 0 {
			z.last = p[n-1]
			z.had = true
			if err != nil && err != io.EOF {
				return n, err
			}
			return n, nil
		}
		if err != io.EOF {
			return n, err
		}
		if err = z.currRc.Close(); err != nil {
			return 0, err
		}
		z.currRc = nil
		if z.had && z.last != '\n' {
			p[0] = '\n'
			return 1, nil
		}
	}
}
func (z *zipStreamReader) Close() error {
	if z.currRc != nil {
		return z.currRc.Close()
	}
	return nil
}
func init() {
	register(".zip", func(file *os.File, _ string) (io.ReadCloser, error) {
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		zr, err := zip.NewReader(file, info.Size())
		if err != nil {
			return nil, err
		}
		return &zipStreamReader{zr: zr}, nil
	})
}
