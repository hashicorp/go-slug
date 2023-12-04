// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
)

func main() {
	w, err := os.Create("../subdir-appears-first.tar.gz")
	if err != nil {
		log.Fatal(err)
	}
	defer w.Close()

	gzipW, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		log.Fatal(err)
	}
	defer gzipW.Close()

	tarW := tar.NewWriter(gzipW)
	defer tarW.Close()

	// The order of headers to write to the output file
	targets := []string{
		"super/duper",
		"super/duper/trooper",
		"super",
		"super/duper/trooper/foo.txt",
	}

	for _, t := range targets {
		info, err := os.Stat(t)
		if err != nil {
			log.Fatal(err)
		}

		header := &tar.Header{
			Format:  tar.FormatUnknown,
			Name:    t,
			ModTime: info.ModTime(),
			Mode:    int64(info.Mode()),
		}

		switch {
		case info.IsDir():
			header.Typeflag = tar.TypeDir
			header.Name += "/"
		default:
			header.Typeflag = tar.TypeReg
			header.Size = info.Size()
		}

		// Write the header first to the archive.
		if err := tarW.WriteHeader(header); err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Added %q, unix nano mtime %d / %d\n", header.Name, info.ModTime().Unix(), info.ModTime().UnixNano())

		if info.IsDir() {
			continue
		}

		f, err := os.Open(t)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		_, err = io.Copy(tarW, f)
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Printf("Copy these values into the
}
