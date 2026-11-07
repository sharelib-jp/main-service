package api

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sharelib-jp/main-service/config"
)

// RegisterWellKnownRoutes exposes public repository metadata and branding.
func RegisterWellKnownRoutes(mux *http.ServeMux, conf *config.Config) {
	mux.HandleFunc("GET /.well-known/nodeinfo", func(w http.ResponseWriter, r *http.Request) {
		metadata := conf.Metadata
		if metadata.NodeAdmins == nil {
			metadata.NodeAdmins = []config.Contact{}
		}
		if metadata.Maintainer == nil {
			metadata.Maintainer = []config.Contact{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if err := json.NewEncoder(w).Encode(struct {
			Version  string          `json:"version"`
			Metadata config.Metadata `json:"metadata"`
		}{Version: "0.1", Metadata: metadata}); err != nil {
			log.Printf("write nodeinfo response: %v", err)
		}
	})
	mux.HandleFunc("GET /.well-known/header", func(w http.ResponseWriter, r *http.Request) {
		path := conf.BrandResources.HeaderPath()
		if path == "" {
			http.NotFound(w, r)
			return
		}
		var contentType string
		switch strings.ToLower(filepath.Ext(path)) {
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		case ".png":
			contentType = "image/png"
		case ".svg":
			contentType = "image/svg+xml"
		default:
			http.Error(w, "unsupported image type", http.StatusUnsupportedMediaType)
			return
		}
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			log.Printf("open header image: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			log.Printf("stat header image: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !info.Mode().IsRegular() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	})
}
