package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	data := `[HOST]
SCHEME = "http://"
HOSTNAME = "localhost"
PORT = ":8082"
[DATABASE]
DATABASE_DRIVER = "sqlite"
DATABASE_DSN = ":memory:"
[METADATA]
NODE_NAME = "test"
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.PORT != ":8082" || c.DATABASE_DSN != ":memory:" || c.Metadata.NodeName != "test" {
		t.Fatalf("unexpected config: %+v", c)
	}
	for _, mutate := range []func(*Config){func(c *Config) { c.DATABASE_DRIVER = "mysql" }, func(c *Config) { c.DATABASE_DSN = " " }, func(c *Config) { c.PORT = ":0" }, func(c *Config) { c.HOSTNAME = "example.com" }} {
		copy := *c
		mutate(&copy)
		if copy.Validate() == nil {
			t.Errorf("accepted invalid config: %+v", copy)
		}
	}
}
