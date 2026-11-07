// Package api registers the main service's HTTP routes.
package api

import (
	"net/http"

	"github.com/sharelib-jp/main-service/config"
	"gorm.io/gorm"
)

// Dependencies holds the shared services available to future API handlers.
type Dependencies struct {
	DB   *gorm.DB
	Site http.Handler
}

// NewRouter registers the public endpoints and provides a place for API routes.
func NewRouter(conf *config.Config, dependencies Dependencies) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterWellKnownRoutes(mux, conf)
	if dependencies.Site != nil {
		mux.Handle("/", dependencies.Site)
	}
	return mux
}
