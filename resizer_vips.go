package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/davidbyttow/govips/v2/vips"
)

type vipsResizer struct{}

func (vipsResizer) Resize(source, destination, format string, width, height, quality int) error {
	image, err := vips.NewImageFromFile(source)
	if err != nil {
		return err
	}
	defer image.Close()

	if image.Width() <= width && (height == 0 || image.Height() <= height) {
		return publishSymlink(source, destination)
	}

	if height == 0 {
		if err := image.Resize(float64(width)/float64(image.Width()), vips.KernelLanczos3); err != nil {
			return err
		}
	} else {
		if err := image.Thumbnail(width, height, vips.InterestingNone); err != nil {
			return err
		}
	}

	var data []byte
	switch format {
	case "jpg", "jpeg":
		params := vips.NewJpegExportParams()
		params.Quality = quality
		data, _, err = image.ExportJpeg(params)
	case "png":
		data, _, err = image.ExportPng(vips.NewPngExportParams())
	case "webp":
		params := vips.NewWebpExportParams()
		params.Quality = quality
		params.Lossless = quality == 100
		data, _, err = image.ExportWebp(params)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
	if err != nil {
		return err
	}

	temporary, err := os.CreateTemp(filepath.Dir(destination), ".yatex-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err = temporary.Write(data); err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(temporaryName, 0o644); err != nil {
		return err
	}
	return os.Rename(temporaryName, destination)
}

func publishSymlink(source, destination string) error {
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".yatex-link-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryName)
		return err
	}
	if err := os.Remove(temporaryName); err != nil {
		return err
	}
	defer os.Remove(temporaryName)

	relativeSource, err := filepath.Rel(filepath.Dir(destination), source)
	if err != nil {
		return err
	}
	if err := os.Symlink(relativeSource, temporaryName); err != nil {
		return err
	}
	return os.Rename(temporaryName, destination)
}
