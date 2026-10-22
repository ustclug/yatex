package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writePNG(t *testing.T, filename string, width, height int) {
	t.Helper()
	file, err := os.Create(filename)
	if !assert.NoError(t, err) {
		return
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.Set(x, y, color.RGBA{R: 30, G: 90, B: 180, A: 255})
		}
	}
	assert.NoError(t, png.Encode(file, img))
	assert.NoError(t, file.Close())
}

func TestVipsResizerSymlinksSourceWithinBoundingBox(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.png")
	destination := filepath.Join(root, "photo@1024x768.png")
	writePNG(t, source, 800, 600)

	assert.NoError(t, (vipsResizer{}).Resize(source, destination, "png", 1024, 768, 0))
	info, err := os.Lstat(destination)
	if assert.NoError(t, err) {
		assert.NotZero(t, info.Mode()&os.ModeSymlink)
	}
	target, err := os.Readlink(destination)
	assert.NoError(t, err)
	assert.Equal(t, "photo.png", target)
}

func TestVipsResizerResizesSourceOutsideBoundingBox(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.png")
	destination := filepath.Join(root, "photo@640x480.png")
	writePNG(t, source, 800, 600)

	assert.NoError(t, (vipsResizer{}).Resize(source, destination, "png", 640, 480, 0))
	info, err := os.Lstat(destination)
	if assert.NoError(t, err) {
		assert.Zero(t, info.Mode()&os.ModeSymlink)
	}
	file, err := os.Open(destination)
	if !assert.NoError(t, err) {
		return
	}
	defer file.Close()
	output, _, err := image.Decode(file)
	if assert.NoError(t, err) {
		assert.Equal(t, image.Rect(0, 0, 640, 480), output.Bounds())
	}
}

func TestVipsResizerWebPQuality100IsLossless(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.png")
	destination := filepath.Join(root, "photo@640x480@100.webp")
	writePNG(t, source, 800, 600)

	assert.NoError(t, (vipsResizer{}).Resize(source, destination, "webp", 640, 480, 100))
	data, err := os.ReadFile(destination)
	if !assert.NoError(t, err) || !assert.GreaterOrEqual(t, len(data), 16) {
		return
	}
	assert.Equal(t, "RIFF", string(data[:4]))
	assert.Equal(t, "WEBP", string(data[8:12]))
	assert.True(t, bytes.Contains(data[12:], []byte("VP8L")), "WebP does not contain a lossless payload")
}
