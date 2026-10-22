package main

import (
	"errors"
	"fmt"
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
		redirect(w, req.urlPath)
		return
	}

	s.resizeMu.Lock()
	defer s.resizeMu.Unlock()

	if isRegularFile(req.target) {
		redirect(w, req.urlPath)
		return
	}
	if isRegularFile(req.outputTarget) {
		if err := publishSymlink(req.outputTarget, req.target); err != nil {
			log.Printf("create default-quality link for %q: %v", req.urlPath, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		redirect(w, req.urlPath)
		return
	}
	source, err := s.findSource(req)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("find source for %q: %v", req.urlPath, err)
		}
		http.NotFound(w, r)
		return
	}
	if err := os.MkdirAll(filepath.Dir(req.target), 0o755); err != nil {
		log.Printf("create output directory for %q: %v", req.urlPath, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if err := s.resizer.Resize(source, req.outputTarget, req.format, req.width, req.height, req.quality); err != nil {
		log.Printf("resize %q from %q: %v", req.urlPath, source, err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if req.target != req.outputTarget {
		if err := publishSymlink(req.outputTarget, req.target); err != nil {
			log.Printf("create default-quality link for %q: %v", req.urlPath, err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	redirect(w, req.urlPath)
}

type imageRequest struct {
	urlPath       string
	target        string
	outputTarget  string
	directory     string
	sourceBase    string
	requestedExt  string
	format        string
	width, height int
	quality       int
}

func (s *service) parseRequest(urlPath string) (imageRequest, bool) {
	if urlPath == "" || !strings.HasPrefix(urlPath, "/") || path.Clean(urlPath) != urlPath {
		return imageRequest{}, false
	}
	name := path.Base(urlPath)
	ext := path.Ext(name)
	if ext == "" {
		return imageRequest{}, false
	}
	format := strings.ToLower(strings.TrimPrefix(ext, "."))
	if _, ok := s.cfg.formatSet[format]; !ok {
		return imageRequest{}, false
	}
	stem := strings.TrimSuffix(name, ext)
	parts := strings.Split(stem, "@")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" {
		return imageRequest{}, false
	}
	if format == "png" && len(parts) != 2 {
		return imageRequest{}, false
	}
	dimension := parts[1]
	if _, ok := s.cfg.dimensionSet[dimension]; !ok {
		return imageRequest{}, false
	}
	width, height, err := parseDimension(dimension)
	if err != nil {
		return imageRequest{}, false
	}
	quality := 0
	qualitySpecified := len(parts) == 3
	if format != "png" {
		quality = s.cfg.defaultQuality
		if qualitySpecified {
			quality, err = strconv.Atoi(parts[2])
			if err != nil {
				return imageRequest{}, false
			}
		}
		if _, ok := s.cfg.qualitySet[quality]; !ok {
			return imageRequest{}, false
		}
	}
	relative := strings.TrimPrefix(urlPath, "/")
	target := filepath.Join(s.cfg.Root, filepath.FromSlash(relative))
	directory := filepath.Dir(target)
	outputTarget := target
	if format != "png" && !qualitySpecified {
		outputName := parts[0] + "@" + dimension + "@" + strconv.Itoa(quality) + ext
		outputTarget = filepath.Join(directory, outputName)
	}
	return imageRequest{
		urlPath: urlPath, target: target, outputTarget: outputTarget, directory: directory,
		sourceBase: parts[0], requestedExt: strings.TrimPrefix(ext, "."),
		format: format, width: width, height: height, quality: quality,
	}, true
}

func (s *service) findSource(req imageRequest) (string, error) {
	exact := filepath.Join(req.directory, req.sourceBase+"."+req.requestedExt)
	if isRegularFile(exact) {
		return exact, nil
	}
	entries, err := os.ReadDir(req.directory)
	if err != nil {
		return "", err
	}
	wantedName := req.sourceBase + "." + req.requestedExt
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(entry.Name(), wantedName) {
			return filepath.Join(req.directory, entry.Name()), nil
		}
	}
	for _, format := range s.cfg.Formats {
		wantedName = req.sourceBase + "." + format
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(entry.Name(), wantedName) {
				return filepath.Join(req.directory, entry.Name()), nil
			}
		}
	}
	return "", os.ErrNotExist
}

func isRegularFile(filename string) bool {
	info, err := os.Stat(filename)
	return err == nil && info.Mode().IsRegular()
}

func redirect(w http.ResponseWriter, urlPath string) {
	w.Header().Set("X-Accel-Redirect", urlPath)
	w.WriteHeader(http.StatusOK)
}

func (r imageRequest) String() string {
	return fmt.Sprintf("%s (%dx%d)", r.urlPath, r.width, r.height)
}
