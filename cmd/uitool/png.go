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
	f, err := os.Create(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "uigolden: savePNG:", err)
		return
	}
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, "uigolden: savePNG:", err)
	}
	_ = f.Close()
}

func loadPNG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return png.Decode(f)
}
