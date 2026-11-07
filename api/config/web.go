package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type WebConfig struct {
	PublicOrigin string `toml:"PUBLIC_ORIGIN"`
}
type RepositoryConfig struct {
	BaseURL string `toml:"BASE_URL"`
	Token   string `toml:"TOKEN"`
}
type SSGConfig struct {
	OutputDir      string `toml:"OUTPUT_DIR"`
	Interval       string `toml:"INTERVAL"`
	GatewayManaged bool   `toml:"GATEWAY_MANAGED"`
}

func (c *Config) PublicOrigin() string {
	if c.Web.PublicOrigin != "" {
		return strings.TrimRight(c.Web.PublicOrigin, "/")
	}
	return c.Origin()
}
func (c *Config) validateWeb() error {
	if origin := c.Web.PublicOrigin; origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || strings.Contains(origin, "#") || u.ForceQuery || (u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "localhost")) {
			return fmt.Errorf("WEB PUBLIC_ORIGIN must be an HTTPS origin (HTTP allowed for localhost)")
		}
	}
	if c.Repository.BaseURL != "" {
		u, err := url.Parse(c.Repository.BaseURL)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || strings.Contains(c.Repository.BaseURL, "#") || u.ForceQuery || (u.Scheme != "http" && u.Scheme != "https") {
			return fmt.Errorf("invalid REPOSITORY BASE_URL")
		}
		if len(c.Repository.Token) < 32 || len(c.Repository.Token) > 256 || strings.ContainsAny(c.Repository.Token, " \t\r\n") {
			return fmt.Errorf("REPOSITORY TOKEN must contain 32 to 256 bytes without whitespace")
		}
	}
	duration, err := time.ParseDuration(c.SSG.Interval)
	if err != nil || duration < 0 || (duration > 0 && duration < time.Second) {
		return fmt.Errorf("SSG INTERVAL must be 0s or at least 1s")
	}
	if c.SSG.OutputDir == "" || strings.ContainsAny(c.SSG.OutputDir, "\x00\r\n{}$") {
		return fmt.Errorf("invalid SSG OUTPUT_DIR")
	}
	absolute, err := filepath.Abs(c.SSG.OutputDir)
	if err != nil {
		return err
	}
	c.SSG.OutputDir = absolute
	return nil
}
