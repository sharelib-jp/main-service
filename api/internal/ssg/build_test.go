package ssg

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/testcatalog"
	"github.com/sharelib-jp/main-service/internal/views"
)

func builder(t *testing.T, managed bool) (*Builder, *testcatalog.Source) {
	t.Helper()
	s := testcatalog.New(testcatalog.Fixture())
	b := &Builder{Source: s, Site: views.Site{Name: "図書館", Origin: "https://example.test"}, Output: t.TempDir(), GatewayManaged: managed}
	return b, s
}
func TestGenerationAtomicityPaginationAndRetention(t *testing.T) {
	b, source := builder(t, false)
	data := testcatalog.Fixture()
	for i := 3; i <= 54; i++ {
		book := data.Books[0]
		book.ID = testcatalog.ID(1000 + i)
		book.Copies = []catalog.Copy{}
		data.Books = append(data.Books, book)
	}
	source.Set(data)
	first, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if Current(b.Output) != first.ID {
		t.Fatal("native generation not published")
	}
	path := filepath.Join(PublicDir(b.Output, first.ID), "author", testcatalog.ID(100), "2.html")
	if _, err = os.Stat(path); err != nil {
		t.Fatal("missing authority pagination", err)
	}
	routesData, err := os.ReadFile(filepath.Join(b.Output, "releases", first.ID, "routes.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(routesData), "/copy/100001") || strings.Contains(string(routesData), "日本語") {
		t.Fatal("unsafe Caddy code mapping")
	}
	source.Fail(errors.New("partial export"))
	if _, err = b.Generate(context.Background()); err == nil || Current(b.Output) != first.ID {
		t.Fatal("failed fetch replaced current generation")
	}
	source.Fail(nil)
	data.Books[0].Authors[0].Authority.ID = testcatalog.ID(999)
	source.Set(data)
	if _, err = b.Generate(context.Background()); err == nil || Current(b.Output) != first.ID {
		t.Fatal("broken references were published")
	}
	source.Set(testcatalog.Fixture())
	second, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(PublicDir(b.Output, second.ID), "author", testcatalog.ID(100), "2.html")); !os.IsNotExist(err) {
		t.Fatal("obsolete page remains")
	}
	third, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if Current(b.Output) != third.ID || readID(b.Output, "previous") != second.ID {
		t.Fatal("bad generation pointers")
	}
	if _, err = os.Stat(filepath.Join(b.Output, "releases", first.ID)); !os.IsNotExist(err) {
		t.Fatal("old generations accumulate")
	}
	guard, err := lock(b.Output)
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Generate(context.Background())
	unlock(guard)
	if !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent generation not excluded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = b.Generate(ctx); err == nil || Current(b.Output) != third.ID {
		t.Fatal("cancellation changed published generation")
	}
}
func TestPublisherFailureRecoveryAndRestart(t *testing.T) {
	b, _ := builder(t, true)
	first, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if Current(b.Output) != "" {
		t.Fatal("managed generation published without Caddy")
	}
	if err = prepareGateway(b.Output); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(b.Output, "control", "active.caddy")
	original, _ := os.ReadFile(active)
	fail := "validate"
	calls := []string{}
	p := Publisher{Output: b.Output, Config: "Caddyfile", Run: func(_ context.Context, _ string, args ...string) error {
		calls = append(calls, args[0])
		if args[0] == fail {
			return fmt.Errorf("injected %s failure", fail)
		}
		return nil
	}}
	if changed, err := p.Publish(context.Background()); err == nil || changed {
		t.Fatal("validation failure accepted")
	}
	if got, _ := os.ReadFile(active); string(got) != string(original) {
		t.Fatal("failed validation did not restore include")
	}
	if len(calls) != 1 || calls[0] != "validate" {
		t.Fatal("reload executed despite validation failure")
	}
	fail = "reload"
	if _, err = p.Publish(context.Background()); err == nil || Current(b.Output) != "" {
		t.Fatal("failed reload advanced publication")
	}
	fail = ""
	if changed, err := p.Publish(context.Background()); err != nil || !changed || Current(b.Output) != first.ID {
		t.Fatalf("publication failed: %v", err)
	}
	second, err := b.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	fail = "reload"
	if _, err = p.Publish(context.Background()); err == nil || Current(b.Output) != first.ID {
		t.Fatal("failed second generation replaced first")
	}
	// A crash between staging an include and validating it must restart the last good one.
	if err = atomicWrite(active, []byte("invalid staged config"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = prepareGateway(b.Output); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(active); !strings.Contains(string(got), first.ID) || strings.Contains(string(got), second.ID) {
		t.Fatal("restart did not restore known-good include")
	}
	fail = ""
	if _, err = p.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	if readID(b.Output, "previous") != first.ID {
		t.Fatal("previous generation not retained")
	}
}
