package main

import (
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type resizer interface {
	Resize(source, destination, format string, width, height, quality int) error
}

type service struct {
	cfg      *config
	resizer  resizer
	resizeMu sync.Mutex
}

func newService(cfg *config, imageResizer resizer) *service {
	return &service{cfg: cfg, resizer: imageResizer}
}

func (s *service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}

	req, ok := s.parseRequest(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	if isRegularFile(req.target) {
		redirect(w, req.redirectPath)
		return
	}

	s.resizeMu.Lock()
	defer s.resizeMu.Unlock()

	if isRegularFile(req.target) {
		redirect(w, req.redirectPath)
		return
	}
	if !isRegularFile(req.source) {
		http.NotFound(w, r)
		return
	}
	if err := os.MkdirAll(filepath.Dir(req.target), 0o755); err != nil {
		serverError(w, "create output directory", req.urlPath, err)
		return
	}
	if err := s.resizer.Resize(req.source, req.target, req.format, req.width, req.height, req.quality); err != nil {
		serverError(w, "resize", req.urlPath, err)
		return
	}
	redirect(w, req.redirectPath)
}

func serverError(w http.ResponseWriter, operation, urlPath string, err error) {
	log.Printf("%s for %q: %v", operation, urlPath, err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

type imageRequest struct {
	urlPath, redirectPath string
	source, target        string
	format                string
	width, height         int
	quality               int
}

func (s *service) parseRequest(urlPath string) (imageRequest, bool) {
	if urlPath == "" || !strings.HasPrefix(urlPath, "/") || path.Clean(urlPath) != urlPath {
		return imageRequest{}, false
	}
	name := path.Base(urlPath)
	at := strings.LastIndexByte(name, '@')
	if at <= 0 || at == len(name)-1 {
		return imageRequest{}, false
	}
	sourceName, parameters := name[:at], name[at+1:]
	sourceFormat := strings.ToLower(strings.TrimPrefix(path.Ext(sourceName), "."))
	if sourceFormat == "" {
		return imageRequest{}, false
	}
	if _, ok := s.cfg.formatSet[sourceFormat]; !ok {
		return imageRequest{}, false
	}
	outputExt := path.Ext(parameters)
	format := sourceFormat
	if outputExt != "" {
		format = strings.ToLower(strings.TrimPrefix(outputExt, "."))
		if _, ok := s.cfg.formatSet[format]; !ok {
			return imageRequest{}, false
		}
	}

	parameterStem := parameters
	if outputExt != "" {
		parameterStem = strings.TrimSuffix(parameters, outputExt)
	}
	parts := strings.Split(parameterStem, "_")
	if len(parts) < 1 || len(parts) > 3 {
		return imageRequest{}, false
	}
	width, ok := parseSuffixedInt(parts[0], 'w')
	if !ok {
		return imageRequest{}, false
	}
	height, quality, next := 0, 0, 1
	if next < len(parts) && strings.HasSuffix(parts[next], "h") {
		height, ok = parseSuffixedInt(parts[next], 'h')
		if !ok {
			return imageRequest{}, false
		}
		next++
	}
	dimension := strconv.Itoa(width)
	if height > 0 {
		dimension += "x" + strconv.Itoa(height)
	}
	if _, ok := s.cfg.dimensionSet[dimension]; !ok {
		return imageRequest{}, false
	}

	qualitySpecified := next < len(parts)
	if format == "png" {
		if qualitySpecified {
			return imageRequest{}, false
		}
	} else {
		quality = s.cfg.defaultQuality
		if qualitySpecified {
			quality, ok = parseSuffixedInt(parts[next], 'q')
			if !ok {
				return imageRequest{}, false
			}
			next++
		}
		if _, ok := s.cfg.qualitySet[quality]; !ok {
			return imageRequest{}, false
		}
	}
	if next != len(parts) {
		return imageRequest{}, false
	}

	canonicalParameters := strconv.Itoa(width) + "w"
	if height > 0 {
		canonicalParameters += "_" + strconv.Itoa(height) + "h"
	}
	if format != "png" {
		canonicalParameters += "_" + strconv.Itoa(quality) + "q"
	}
	canonicalName := sourceName + "@" + canonicalParameters + "." + format
	directoryURL := path.Dir(urlPath)
	sourceURL := path.Join(directoryURL, sourceName)
	outputURL := path.Join(s.cfg.OutputPrefix, strings.TrimPrefix(path.Join(directoryURL, canonicalName), "/"))
	return imageRequest{
		urlPath: urlPath, redirectPath: outputURL,
		source: filepath.Join(s.cfg.Root, filepath.FromSlash(sourceURL)),
		target: filepath.Join(s.cfg.Root, filepath.FromSlash(outputURL)), format: format,
		width: width, height: height, quality: quality,
	}, true
}

func parseSuffixedInt(value string, suffix byte) (int, bool) {
	if len(value) < 2 || value[len(value)-1] != suffix {
		return 0, false
	}
	n, err := strconv.Atoi(value[:len(value)-1])
	return n, err == nil && n > 0
}

func isRegularFile(filename string) bool {
	info, err := os.Stat(filename)
	return err == nil && info.Mode().IsRegular()
}

func redirect(w http.ResponseWriter, urlPath string) {
	w.Header().Set("X-Accel-Redirect", urlPath)
	w.WriteHeader(http.StatusOK)
}
