package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

type config struct {
	Root       string   `yaml:"root"`
	Formats    []string `yaml:"formats"`
	Dimensions []any    `yaml:"dimensions"`
	Qualities  []int    `yaml:"qualities"`

	dimensionSet   map[string]struct{}
	formatSet      map[string]struct{}
	qualitySet     map[int]struct{}
	defaultQuality int
}

func loadConfig(filename string) (*config, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.prepare(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *config) prepare() error {
	if c.Root == "" {
		return fmt.Errorf("config: root is required")
	}
	if len(c.Formats) == 0 {
		return fmt.Errorf("config: at least one format is required")
	}
	if len(c.Dimensions) == 0 {
		return fmt.Errorf("config: at least one dimension is required")
	}
	if len(c.Qualities) == 0 {
		return fmt.Errorf("config: at least one quality is required")
	}

	c.formatSet = make(map[string]struct{}, len(c.Formats))
	for i, format := range c.Formats {
		format = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), "."))
		if format == "" || strings.ContainsAny(format, `/\`) {
			return fmt.Errorf("config: invalid format %q", c.Formats[i])
		}
		c.Formats[i] = format
		c.formatSet[format] = struct{}{}
	}

	c.dimensionSet = make(map[string]struct{}, len(c.Dimensions))
	for _, raw := range c.Dimensions {
		var value string
		switch v := raw.(type) {
		case string:
			value = v
		case uint64:
			value = strconv.FormatUint(v, 10)
		case int64:
			value = strconv.FormatInt(v, 10)
		case int:
			value = strconv.Itoa(v)
		default:
			return fmt.Errorf("config: invalid dimension %v", raw)
		}
		if _, _, err := parseDimension(value); err != nil {
			return fmt.Errorf("config: %w", err)
		}
		c.dimensionSet[value] = struct{}{}
	}

	c.qualitySet = make(map[int]struct{}, len(c.Qualities))
	for _, quality := range c.Qualities {
		if quality < 1 || quality > 100 {
			return fmt.Errorf("config: invalid quality %d", quality)
		}
		c.qualitySet[quality] = struct{}{}
	}
	c.defaultQuality = c.Qualities[0]
	return nil
}

func parseDimension(value string) (int, int, error) {
	parts := strings.Split(value, "x")
	if len(parts) > 2 || len(parts) == 0 {
		return 0, 0, fmt.Errorf("invalid dimension %q", value)
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil || width <= 0 {
		return 0, 0, fmt.Errorf("invalid dimension %q", value)
	}
	height := 0
	if len(parts) == 2 {
		height, err = strconv.Atoi(parts[1])
		if err != nil || height <= 0 {
			return 0, 0, fmt.Errorf("invalid dimension %q", value)
		}
	}
	return width, height, nil
}
