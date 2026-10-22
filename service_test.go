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
	quality                     int
}

type fakeResizer struct {
	mu    sync.Mutex
	calls []resizeCall
	err   error
}

func (f *fakeResizer) Resize(source, destination, format string, width, height, quality int) error {
	f.mu.Lock()
	f.calls = append(f.calls, resizeCall{source, destination, format, width, height, quality})
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(destination, []byte("thumbnail"), 0o644)
}

func testConfig(t *testing.T, root string) *config {
	t.Helper()
	cfg := &config{
		Root: root, OutputPrefix: "/.yatex", Formats: []string{"webp", "png", "jpeg", "jpg"},
		Dimensions: []any{uint64(640), "640x480"}, Qualities: []int{75, 82, 100},
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
	target := filepath.Join(root, ".yatex", "photo.jpg@640w_75q.jpg")
	assert.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	assert.NoError(t, os.WriteFile(target, []byte("existing"), 0o644))
	resizer := &fakeResizer{}
	recorder := request(t, newService(testConfig(t, root), resizer), "/photo.jpg@640w_75q.jpg")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/.yatex/photo.jpg@640w_75q.jpg", recorder.Header().Get("X-Accel-Redirect"))
	assert.Empty(t, recorder.Body.String())
	assert.Empty(t, resizer.calls)
}

func TestGeneratesFromExactSourceName(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "nested", "photo.JPG")
	assert.NoError(t, os.MkdirAll(filepath.Dir(source), 0o755))
	assert.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
	resizer := &fakeResizer{}
	recorder := request(t, newService(testConfig(t, root), resizer), "/nested/photo.JPG@640w")
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/.yatex/nested/photo.JPG@640w_75q.jpg", recorder.Header().Get("X-Accel-Redirect"))
	if !assert.Len(t, resizer.calls, 1) {
		return
	}
	call := resizer.calls[0]
	assert.Equal(t, source, call.source)
	assert.Equal(t, "jpg", call.format)
	assert.Equal(t, 640, call.width)
	assert.Zero(t, call.height)
	assert.Equal(t, 75, call.quality)
	assert.Equal(t, filepath.Join(root, ".yatex", "nested", "photo.JPG@640w_75q.jpg"), call.destination)
	info, err := os.Lstat(call.destination)
	if assert.NoError(t, err) {
		assert.False(t, info.Mode().Type() == os.ModeSymlink)
	}
}

func TestSourceNameRemainsIntact(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo@original.jpg")
	assert.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
	resizer := &fakeResizer{}

	recorder := request(t, newService(testConfig(t, root), resizer), "/photo@original.jpg@640w_82q.webp")

	assert.Equal(t, http.StatusOK, recorder.Code)
	if assert.Len(t, resizer.calls, 1) {
		assert.Equal(t, source, resizer.calls[0].source)
		assert.Equal(t, filepath.Join(root, ".yatex", "photo@original.jpg@640w_82q.webp"), resizer.calls[0].destination)
	}
}

func TestGeneratesExplicitQualityVariant(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.jpg")
	assert.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
	resizer := &fakeResizer{}

	recorder := request(t, newService(testConfig(t, root), resizer), "/photo.jpg@640w_82q.webp")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/.yatex/photo.jpg@640w_82q.webp", recorder.Header().Get("X-Accel-Redirect"))
	if assert.Len(t, resizer.calls, 1) {
		assert.Equal(t, filepath.Join(root, ".yatex", "photo.jpg@640w_82q.webp"), resizer.calls[0].destination)
		assert.Equal(t, "webp", resizer.calls[0].format)
		assert.Equal(t, 82, resizer.calls[0].quality)
	}
	_, err := os.Lstat(filepath.Join(root, ".yatex", "photo.jpg@640w.webp"))
	assert.True(t, os.IsNotExist(err))
}

func TestOmittedQualityAndExtensionReuseCanonicalVariant(t *testing.T) {
	root := t.TempDir()
	defaultVariant := filepath.Join(root, ".yatex", "photo.webp@640w_75q.webp")
	assert.NoError(t, os.MkdirAll(filepath.Dir(defaultVariant), 0o755))
	assert.NoError(t, os.WriteFile(defaultVariant, []byte("existing"), 0o644))
	resizer := &fakeResizer{}

	recorder := request(t, newService(testConfig(t, root), resizer), "/photo.webp@640w")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/.yatex/photo.webp@640w_75q.webp", recorder.Header().Get("X-Accel-Redirect"))
	assert.Empty(t, resizer.calls)
}

func TestOmittedAndExplicitExtensionsResolveToSameArtifact(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(root, "photo.jpg"), []byte("source"), 0o644))
	resizer := new(fakeResizer)
	service := newService(testConfig(t, root), resizer)

	omitted := request(t, service, "/photo.jpg@640w_480h")
	explicit := request(t, service, "/photo.jpg@640w_480h.jpg")

	canonical := "/.yatex/photo.jpg@640w_480h_75q.jpg"
	assert.Equal(t, http.StatusOK, omitted.Code)
	assert.Equal(t, http.StatusOK, explicit.Code)
	assert.Equal(t, canonical, omitted.Header().Get("X-Accel-Redirect"))
	assert.Equal(t, canonical, explicit.Header().Get("X-Accel-Redirect"))
	assert.Len(t, resizer.calls, 1)
}

func TestConfiguredOutputPrefixControlsStorageAndRedirect(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, os.MkdirAll(filepath.Join(root, "images"), 0o755))
	source := filepath.Join(root, "images", "photo.jpg")
	assert.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
	cfg := testConfig(t, root)
	cfg.OutputPrefix = "/cache/thumbs"
	resizer := new(fakeResizer)

	recorder := request(t, newService(cfg, resizer), "/images/photo.jpg@640w")

	canonical := "/cache/thumbs/images/photo.jpg@640w_75q.jpg"
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, canonical, recorder.Header().Get("X-Accel-Redirect"))
	if assert.Len(t, resizer.calls, 1) {
		assert.Equal(t, source, resizer.calls[0].source)
		assert.Equal(t, filepath.Join(root, "cache", "thumbs", "images", "photo.jpg@640w_75q.jpg"), resizer.calls[0].destination)
	}
}

func TestPNGGeneratesWithoutQuality(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "photo.png")
	assert.NoError(t, os.WriteFile(source, []byte("source"), 0o644))
	resizer := &fakeResizer{}

	recorder := request(t, newService(testConfig(t, root), resizer), "/photo.png@640w")

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "/.yatex/photo.png@640w.png", recorder.Header().Get("X-Accel-Redirect"))
	if assert.Len(t, resizer.calls, 1) {
		assert.Equal(t, filepath.Join(root, ".yatex", "photo.png@640w.png"), resizer.calls[0].destination)
		assert.Zero(t, resizer.calls[0].quality)
	}
}

func TestRejectsInvalidRequests(t *testing.T) {
	service := newService(testConfig(t, t.TempDir()), &fakeResizer{})
	for _, target := range []string{
		"/photo.jpg@123w.jpg", "/photo.jpg@640w.gif", "/photo.jpg",
		"/photo.jpg@640w_60q.jpg", "/photo.png@640w_75q.png",
		"/photo.jpg@640w_480h_75q_extra.jpg", "/photo.jpg@480h_640w.jpg",
		"/photo.jpg@640w_75q_480h.jpg",
	} {
		recorder := request(t, service, target)
		assert.Equal(t, http.StatusNotFound, recorder.Code, target)
	}
}

func TestMissingSourceReturns404(t *testing.T) {
	recorder := request(t, newService(testConfig(t, t.TempDir()), &fakeResizer{}), "/photo.png@640w_480h.png")
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
			assert.Equal(t, http.StatusOK, request(t, service, "/photo.jpg@640w").Code)
		}()
	}
	wait.Wait()
	assert.Len(t, resizer.calls, 1)
}
