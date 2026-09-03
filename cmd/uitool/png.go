package main

// Shared PNG helpers for the uitool subcommands (previously duplicated
// between cmd/monkey and cmd/uigolden).

import (
	"fmt"
	"image"
	"image/png"
	"os"
)

func savePNG(path string, img image.Image) {
	f, err := os.Create(path) //nolint:gosec // the path is the file this command was told to read or write
	if err != nil {
		fmt.Fprintln(os.Stderr, "uigolden: savePNG:", err)
		return
	}
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "uigolden: savePNG:", err)
	}
	_ = f.Close() //nolint:errcheck
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path) //nolint:gosec // the path is the file this command was told to read or write
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }() //nolint:errcheck
	return png.Decode(f)
}
