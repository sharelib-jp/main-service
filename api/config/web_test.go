package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWebConfigurationAndPublicOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `[HOST]
SCHEME="https://"
HOSTNAME="library.example.test"
PORT=":8082"
[WEB]
PUBLIC_ORIGIN="https://library.example.test"
[REPOSITORY]
BASE_URL="http://repository:8081"
TOKEN="` + strings.Repeat("t", 32) + `"
[SSG]
OUTPUT_DIR="./generated"
INTERVAL="5m"
GATEWAY_MANAGED=true
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicOrigin() != "https://library.example.test" || c.PORT != ":8082" || !filepath.IsAbs(c.SSG.OutputDir) || !c.SSG.GatewayManaged {
		t.Fatalf("bad configuration: %+v", c)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.Web.PublicOrigin = "https://example.test/path" }, func(c *Config) { c.Web.PublicOrigin = "http://example.test" }, func(c *Config) { c.Repository.BaseURL = "http://user:secret@repository" }, func(c *Config) { c.Repository.Token = "" }, func(c *Config) { c.SSG.Interval = "-1s" }, func(c *Config) { c.SSG.OutputDir = "/tmp/{$INJECTION}" }} {
		copy := *c
		mutate(&copy)
		if copy.Validate() == nil {
			t.Fatal("invalid web configuration accepted")
		}
	}
}
