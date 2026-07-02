package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// cachedProxy fetches upstream content once and serves it from a disk cache
// afterwards, so browsers only ever need to reach this server (fonts, Typst
// Universe packages).
var proxyHTTP = &http.Client{Timeout: 120 * time.Second}

func (s *Server) cachedProxy(w http.ResponseWriter, r *http.Request, upstreamURL, cacheDir, contentType string) {
	sum := sha256.Sum256([]byte(upstreamURL))
	cachePath := filepath.Join(s.Cfg.DataDir, cacheDir, hex.EncodeToString(sum[:8])+"_"+filepath.Base(upstreamURL))

	if data, err := os.ReadFile(cachePath); err == nil {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		_, _ = w.Write(data)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, upstreamURL, nil)
	if err != nil {
		fail(w, err)
		return
	}
	resp, err := proxyHTTP.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "upstream unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 200<<20))
	if err != nil {
		fail(w, err)
		return
	}
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	tmp := cachePath + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, cachePath)
	}
	if contentType == "" {
		contentType = resp.Header.Get("Content-Type")
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

// handleFontProxy serves typst.ts preview font assets from
// jsdelivr (typst/typst-assets, typst/typst-dev-assets), disk-cached.
func (s *Server) handleFontProxy(w http.ResponseWriter, r *http.Request) {
	repo := chi.URLParam(r, "repo")
	file := chi.URLParam(r, "file")
	if (repo != "typst-assets" && repo != "typst-dev-assets") ||
		strings.Contains(file, "/") || strings.Contains(file, "..") {
		writeErr(w, http.StatusBadRequest, "invalid font path")
		return
	}
	upstream := "https://cdn.jsdelivr.net/gh/typst/" + repo + "@v0.13.1/files/fonts/" + file
	s.cachedProxy(w, r, upstream, "font-cache", "font/ttf")
}
