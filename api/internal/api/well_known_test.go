package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sharelib-jp/main-service/config"
)

func TestNodeInfoUsesPublicMetadata(t *testing.T) {
	conf := &config.Config{Metadata: config.Metadata{
		NodeName: "開発図書館", ApplicationName: "リポジトリ管理サーバ", NodeDomain: "library.example",
	}}
	response := httptest.NewRecorder()
	NewRouter(conf, Dependencies{}).ServeHTTP(response, httptest.NewRequest("GET", "/.well-known/nodeinfo", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		Version  string `json:"version"`
		Metadata struct {
			NodeName        string           `json:"nodeName"`
			ApplicationName string           `json:"applicationName"`
			NodeDomain      string           `json:"nodeDomain"`
			NodeAdmins      []config.Contact `json:"nodeAdmins"`
			Maintainer      []config.Contact `json:"maintainer"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Version != "0.1" || body.Metadata.NodeName != "開発図書館" || body.Metadata.ApplicationName != "リポジトリ管理サーバ" || body.Metadata.NodeDomain != "library.example" || body.Metadata.NodeAdmins == nil || body.Metadata.Maintainer == nil {
		t.Fatalf("unexpected metadata: %+v", body)
	}
	if got := response.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("unexpected content type %q", got)
	}
}

func TestHeaderImage(t *testing.T) {
	for _, tc := range []struct {
		name, suffix, mime string
		write              bool
		status             int
	}{
		{"svg", ".svg", "image/svg+xml", true, http.StatusOK},
		{"png", ".png", "image/png", true, http.StatusOK},
		{"jpeg", ".jpeg", "image/jpeg", true, http.StatusOK},
		{"unsupported", ".gif", "", true, http.StatusUnsupportedMediaType},
		{"missing", ".svg", "", false, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "logo"+tc.suffix)
			if tc.write {
				if err := os.WriteFile(path, []byte("image-data"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			conf := &config.Config{BrandResources: config.BrandResources{Header: path}}
			response := httptest.NewRecorder()
			NewRouter(conf, Dependencies{}).ServeHTTP(response, httptest.NewRequest("GET", "/.well-known/header", nil))
			if response.Code != tc.status {
				t.Fatalf("status = %d, want %d", response.Code, tc.status)
			}
			if tc.status == http.StatusOK {
				if response.Body.String() != "image-data" || response.Header().Get("Content-Type") != tc.mime {
					t.Fatalf("unexpected image response: %q, %q", response.Body.String(), response.Header().Get("Content-Type"))
				}
				if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
					t.Fatal("missing image security headers")
				}
			}
		})
	}
}

func TestWellKnownRoutesDoNotPretendToBeOIDCProvider(t *testing.T) {
	router := NewRouter(&config.Config{}, Dependencies{})
	for _, path := range []string{"/.well-known/header", "/.well-known/openid-configuration", "/.well-known/oauth-authorization-server", "/api/.well-known/nodeinfo"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d", path, response.Code)
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", "/.well-known/nodeinfo", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected method status: %d", response.Code)
	}
}
