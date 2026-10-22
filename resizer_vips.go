package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/davidbyttow/govips/v2/vips"
)

type vipsResizer struct{}

func (vipsResizer) Resize(source, destination, format string, width, height int) error {
	image, err := vips.NewImageFromFile(source)
	if err != nil {
		return err
	}
	defer image.Close()

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
		data, _, err = image.ExportJpeg(vips.NewJpegExportParams())
	case "png":
		data, _, err = image.ExportPng(vips.NewPngExportParams())
	case "webp":
		data, _, err = image.ExportWebp(vips.NewWebpExportParams())
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
