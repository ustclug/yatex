package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

type resizeCall struct {
	source, destination, format string
	width, height               int
}

type fakeResizer struct {
	mu    sync.Mutex
	calls []resizeCall
	err   error
}

func (f *fakeResizer) Resize(source, destination, format string, width, height int) error {
	f.mu.Lock()
	f.calls = append(f.calls, resizeCall{source, destination, format, width, height})
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(destination, []byte("thumbnail"), 0o644)
}

func testConfig(t *testing.T, root string) *config {
	t.Helper()
	cfg := &config{
		Root: root, Formats: []string{"webp", "png", "jpeg", "jpg"},
		Dimensions: []any{uint64(640), "640x480"}, Qualities: []int{60, 80},
	}
	if !assert.NoError(t, cfg.prepare()) {
		return nil
	}
	return cfg
}

func request(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func TestExistingThumbnailRedirectsWithoutResize(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(root, "photo@640.jpg"), []byte("existing"), 0o644))
	resizer := &fakeResizer{}
	recorder := request(t, newService(testConfig(t, root), resizer), "/photo@640.jpg")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/photo@640.jpg", recorder.Header().Get("X-Accel-Redirect"))
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, resizer.calls)
}

func TestGeneratesThumbnailFromExactSuffix(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "nested", "photo.JPG")
	assert.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	assert.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
	resizer := &fakeResizer{}
	recorder := request(t, newService(testConfig(t, root), resizer), "/nested/photo@640.JPG")
	assert.Equal(t, http.StatusOK, recorder.Code)
	if !assert.Len(t, resizer.calls, 1) {
		return
	}
	call := resizer.calls[0]
	assert.Equal(t, source, call.source)
	assert.Equal(t, "jpg", call.format)
	assert.Equal(t, 640, call.width)
	assert.Zero(t, call.height)
}

func TestSourceSearchPrecedence(t *testing.T) {
	t.Run("requested suffix case insensitive", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"photo.JpG", "photo.webp"} {
			assert.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o644))
		}
		resizer := &fakeResizer{}
		recorder := request(t, newService(testConfig(t, root), resizer), "/photo@640.jpg")
		assert.Equal(t, http.StatusOK, recorder.Code)
		if assert.Len(t, resizer.calls, 1) {
			assert.Equal(t, "photo.JpG", filepath.Base(resizer.calls[0].source))
		}
	})

	t.Run("configured format order", func(t *testing.T) {
		root := t.TempDir()
		for _, name := range []string{"photo.PNG", "photo.WeBp"} {
			assert.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(name), 0o644))
		}
		resizer := &fakeResizer{}
		recorder := request(t, newService(testConfig(t, root), resizer), "/photo@640.jpg")
		assert.Equal(t, http.StatusOK, recorder.Code)
		if assert.Len(t, resizer.calls, 1) {
			assert.Equal(t, "photo.WeBp", filepath.Base(resizer.calls[0].source))
		}
	})
}

func TestRejectsInvalidRequests(t *testing.T) {
	service := newService(testConfig(t, t.TempDir()), &fakeResizer{})
	for _, target := range []string{
		"/photo@123.jpg", "/photo@640.gif", "/photo.jpg", "/photo@640.jpg/extra",
	} {
		recorder := request(t, service, target)
		assert.Equal(t, http.StatusNotFound, recorder.Code, target)
	}
}

func TestMissingSourceReturns404(t *testing.T) {
	recorder := request(t, newService(testConfig(t, t.TempDir()), &fakeResizer{}), "/photo@640x480.png")
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestConcurrentRequestsGenerateOnce(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("source"), 0o644))
	resizer := &fakeResizer{}
	service := newService(testConfig(t, root), resizer)
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			assert.Equal(t, http.StatusOK, request(t, service, "/photo@640.jpg").Code)
		}()
	}
	wait.Wait()
	assert.Len(t, resizer.calls, 1)
}
