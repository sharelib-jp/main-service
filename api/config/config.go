package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Contact struct {
	Name  string `toml:"name" json:"name"`
	Email string `toml:"email" json:"email"`
}
type Metadata struct {
	NodeName        string    `toml:"NODE_NAME" json:"nodeName"`
	ApplicationName string    `toml:"APPLICATION_NAME" json:"applicationName"`
	NodeDomain      string    `toml:"NODE_DOMAIN" json:"nodeDomain"`
	NodeAdmins      []Contact `toml:"NODE_ADMINS" json:"nodeAdmins"`
	Maintainer      []Contact `toml:"MAINTAINER" json:"maintainer"`
}
type BrandResources struct {
	Header     string `toml:"HEADER"`
	HeaderLogo string `toml:"HEADER_LOGO"`
}

func (b BrandResources) HeaderPath() string {
	if b.Header != "" {
		return b.Header
	}
	return b.HeaderLogo
}

type Config struct {
	Web             WebConfig        `toml:"WEB"`
	Repository      RepositoryConfig `toml:"REPOSITORY"`
	SSG             SSGConfig        `toml:"SSG"`
	SCHEME          string
	HOSTNAME        string
	PORT            string
	DATABASE_DRIVER string
	DATABASE_DSN    string
	Metadata        Metadata       `toml:"METADATA"`
	BrandResources  BrandResources `toml:"BRANDRESOURCES"`
}

func LoadConfig(paths ...string) (*Config, error) {
	if len(paths) > 1 {
		return nil, fmt.Errorf("at most one config path may be provided")
	}
	path := "./config.toml"
	if len(paths) == 1 {
		path = paths[0]
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{SSG: SSGConfig{OutputDir: "./generated", Interval: "5m"}, DATABASE_DRIVER: "sqlite", DATABASE_DSN: "main.db?_busy_timeout=5000&_foreign_keys=on&_txlock=immediate"}
	if _, err := toml.Decode(string(data), c); err != nil {
		return nil, err
	}
	var sections struct {
		Host     struct{ Scheme, Hostname, Port *string } `toml:"HOST"`
		Database struct {
			Driver *string `toml:"DATABASE_DRIVER"`
			DSN    *string `toml:"DATABASE_DSN"`
		} `toml:"DATABASE"`
	}
	if _, err := toml.Decode(string(data), &sections); err != nil {
		return nil, err
	}
	for _, f := range []struct {
		src *string
		dst *string
	}{{sections.Host.Scheme, &c.SCHEME}, {sections.Host.Hostname, &c.HOSTNAME}, {sections.Host.Port, &c.PORT}, {sections.Database.Driver, &c.DATABASE_DRIVER}, {sections.Database.DSN, &c.DATABASE_DSN}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}
func (c *Config) Origin() string { return c.SCHEME + c.HOSTNAME + c.PORT }
func (c *Config) Validate() error {
	switch c.DATABASE_DRIVER {
	case "", "sqlite", "postgres":
	default:
		return fmt.Errorf("DATABASE_DRIVER must be sqlite or postgres")
	}
	u, err := url.Parse(c.Origin())
	if err != nil || c.HOSTNAME == "" || u.Hostname() != c.HOSTNAME || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid application origin")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && u.Hostname() == "localhost") {
		return fmt.Errorf("application origin requires HTTPS (HTTP is allowed for localhost)")
	}
	port, err := strconv.Atoi(strings.TrimPrefix(c.PORT, ":"))
	if err != nil || !strings.HasPrefix(c.PORT, ":") || port < 1 || port > 65535 || strings.TrimSpace(c.DATABASE_DSN) == "" {
		return fmt.Errorf("PORT and DATABASE_DSN are required")
	}
	return c.validateWeb()
}
