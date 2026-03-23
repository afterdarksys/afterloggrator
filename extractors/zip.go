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
}

func (z *zipStreamReader) Read(p []byte) (int, error) {
	if z.currRc == nil {
		if z.index >= len(z.zr.File) {
			return 0, io.EOF
		}
		rc, err := z.zr.File[z.index].Open()
		if err != nil {
			z.index++
			return z.Read(p) // skip broken file
		}
		z.currRc = rc
		z.index++
	}

	n, err := z.currRc.Read(p)
	if n > 0 {
		return n, nil
	}
	if err == io.EOF {
		z.currRc.Close()
		z.currRc = nil
		return z.Read(p)
	}
	return n, err
}

func (z *zipStreamReader) Close() error {
	if z.currRc != nil {
		z.currRc.Close()
	}
	return nil
}

func init() {
	register(".zip", func(file *os.File, filename string) (io.ReadCloser, error) {
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		
		zr, err := zip.NewReader(file, info.Size())
		if err != nil {
			return nil, err
		}

		return &zipStreamReader{
			zr: zr,
		}, nil
	})
}
