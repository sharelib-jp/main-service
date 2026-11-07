package ssg_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/site"
	"github.com/sharelib-jp/main-service/internal/ssg"
	"github.com/sharelib-jp/main-service/internal/testcatalog"
	"github.com/sharelib-jp/main-service/internal/views"
)

func port(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}
func TestCaddyPublicationAndFallback(t *testing.T) {
	if _, err := exec.LookPath("caddy"); err != nil {
		t.Skip("caddy is not installed")
	}
	output := t.TempDir()
	source := testcatalog.New(testcatalog.Fixture())
	httpPort, adminPort := port(t), port(t)
	origin := fmt.Sprintf("http://localhost:%d", httpPort)
	website := views.Site{Name: "図書館", Origin: origin}
	handler := site.New(source, website, output)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Backend", "true")
		handler.ServeHTTP(w, r)
	}))
	defer backend.Close()
	b := ssg.Builder{Source: source, Site: website, Output: output, GatewayManaged: true}
	first, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(t.TempDir(), "Caddyfile")
	text := fmt.Sprintf("{\n admin localhost:%d\n auto_https off\n persist_config off\n}\n%s {\n route {\n import %q\n reverse_proxy %s\n }\n}\n", adminPort, origin, filepath.Join(output, "control", "active.caddy"), strings.TrimPrefix(backend.URL, "http://"))
	if err = os.WriteFile(config, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- ssg.RunGateway(ctx, config, output) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("gateway shutdown: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("gateway did not stop")
		}
	})
	waitPublished := func(id string) {
		t.Helper()
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			if ssg.Current(output) == id {
				return
			}
			select {
			case err := <-done:
				t.Fatalf("gateway exited: %v", err)
			default:
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("generation %s was not published", id)
	}
	waitPublished(first.ID)
	client := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path string, status int) (string, http.Header) {
		t.Helper()
		res, err := client.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != status {
			t.Fatalf("%s: %d %s", path, res.StatusCode, body)
		}
		return string(body), res.Header
	}
	bookURL := views.BookURL(testcatalog.ID(1))
	_, headers := get(bookURL, 200)
	if headers.Get("X-Test-Backend") != "" {
		t.Fatal("book was proxied instead of served statically")
	}
	_, headers = get(views.JSONURL(testcatalog.ID(1)), 200)
	if !strings.HasPrefix(headers.Get("Content-Type"), "application/json") {
		t.Fatal("extensionless JSON MIME type")
	}
	_, headers = get("/copy/100001", 302)
	if headers.Get("Location") != bookURL+"#copy-"+testcatalog.ID(10) || headers.Get("X-Test-Backend") != "" {
		t.Fatal("generated copy redirect did not run in Caddy")
	}
	get(views.CopyURL("日本語 /?#{x}"), 302)
	_, headers = get(views.Asset("style.css"), 200)
	if !strings.Contains(headers.Get("Cache-Control"), "immutable") {
		t.Fatal("assets not cached immutably")
	}
	_, headers = get("/search", 200)
	if headers.Get("X-Test-Backend") != "true" {
		t.Fatal("search was not proxied")
	}
	for _, name := range []string{"book.svg", "building.svg", "calendar.svg", "external.svg", "pen.svg", "piggy-bank.svg", "rulers.svg", "sort-down.svg", "copy.js"} {
		_, headers = get(views.Asset(name), 200)
		if headers.Get("X-Test-Backend") != "" {
			t.Fatalf("asset %s was not served by Caddy", name)
		}
	}
	get("/control/published", 404)
	data := testcatalog.Fixture()
	data.Books[0].Title = "変更後のタイトル"
	data.Books[0].Copies[0].ManagementCode = "100002"
	source.Set(data)
	second, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	waitPublished(second.ID)
	body, _ := get(bookURL, 200)
	if !strings.Contains(body, "変更後のタイトル") {
		t.Fatal("HTML generation not switched")
	}
	get("/copy/100001", 404)
	get("/copy/100002", 302)
	source.Fail(catalog.ErrUnavailable)
	get(bookURL, 200)
	get("/search", 503)
}
