package core

import (
	"image"
	"image/png"
	"os"
)

// WritePNG saves a frame.
func WritePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
