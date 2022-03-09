package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
)

func hasFinaliser(fn string) (b bool, err error) {
	f, err := os.Open(fn)
	if err != nil {
		return
	}

	defer f.Close()

	decompressor, err := gzip.NewReader(f)
	if err != nil {
		return
	}

	defer decompressor.Close()

	tr := tar.NewReader(decompressor)
	var header *tar.Header
	for {
		header, err = tr.Next()

		switch {
		case err == io.EOF:
			err = nil

			return
		case err != nil:
			return
		case header == nil:
			continue
		}

		if header.Name == ".trigger" && header.Typeflag == tar.TypeReg {
			b = true

			break
		}
	}

	return
}
