package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yml")
	contents := "root: /srv/data\noutput_prefix: /.yatex\nformats: [webp, JPG]\ndimensions: [640, 640x480]\nqualities: [60, 80]\n"
	assert.NoError(t, os.WriteFile(filename, []byte(contents), 0o644))
	cfg, err := loadConfig(filename)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "/srv/data", cfg.Root)
	assert.Equal(t, "/.yatex", cfg.OutputPrefix)
	assert.Equal(t, "jpg", cfg.Formats[1])
	assert.Equal(t, 60, cfg.defaultQuality)
	if _, ok := cfg.dimensionSet["640"]; !ok {
		assert.Fail(t, "numeric dimension was not loaded")
	}
}
