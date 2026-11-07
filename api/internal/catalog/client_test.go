package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadOnlyWireContractAndFailures(t *testing.T) {
	token := strings.Repeat("t", 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token || r.Method != "GET" {
			t.Error("bad service authentication")
		}
		switch r.URL.Path {
		case "/internal/catalog/snapshot":
			fmt.Fprint(w, `{"schemaVersion":1,"books":[],"authors":[],"publishers":[]}`)
		case "/internal/catalog/books":
			if r.URL.Query().Get("q") != "日本語 & ?" || r.URL.Query().Get("publisherId") == "" {
				t.Error("query lost")
			}
			fmt.Fprint(w, `{"items":[{"id":"00000000000000000000000001","title":"本","reference":{"LargeIdentifier":9007199254740993},"authors":[{"authority":{"id":"00000000000000000000000100","title":"著者"},"role":"著","position":1}],"copies":[]}],"total":1,"page":1,"pageSize":20}`)
		case "/internal/catalog/books/missing":
			w.WriteHeader(404)
		case "/internal/catalog/books/error":
			w.WriteHeader(503)
		case "/internal/catalog/books/truncated":
			fmt.Fprint(w, `{"title":"partial"`)
		default:
			w.Header().Set("Location", "https://example.com/")
			w.WriteHeader(302)
		}
	}))
	defer server.Close()
	c := NewClient(server.URL, token)
	if snapshot, err := c.Snapshot(context.Background()); err != nil || snapshot.SchemaVersion != 1 {
		t.Fatal(err)
	}
	page, err := c.Search(context.Background(), SearchOptions{Query: "日本語 & ?", PublisherID: "00000000000000000000000200", Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || page.Items[0].Authors[0].Authority.Title != "著者" {
		t.Fatalf("wire contract: %+v %v", page, err)
	}
	if page.Items[0].CreatedAt != nil {
		t.Fatal("older response must leave creation time unset")
	}
	if fmt.Sprint(page.Items[0].Reference["LargeIdentifier"]) != "9007199254740993" {
		t.Fatal("reference identifier lost precision")
	}
	for _, test := range []struct {
		id  string
		err error
	}{{"missing", ErrNotFound}, {"error", ErrUnavailable}, {"truncated", ErrUnavailable}, {"redirect", ErrUnavailable}} {
		if _, err := c.Book(context.Background(), test.id); !errors.Is(err, test.err) {
			t.Errorf("%s: %v", test.id, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Snapshot(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatal("cancellation not handled")
	}
}
