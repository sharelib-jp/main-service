package site_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/site"
	"github.com/sharelib-jp/main-service/internal/ssg"
	"github.com/sharelib-jp/main-service/internal/testcatalog"
	"github.com/sharelib-jp/main-service/internal/views"
)

func request(t *testing.T, h http.Handler, path string, hx bool, status int) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	if hx {
		r.Header.Set("HX-Request", "true")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != status {
		t.Fatalf("%s: got %d want %d: %s", path, w.Code, status, w.Body.String())
	}
	return w
}
func TestSemanticPagesAndDynamicFallback(t *testing.T) {
	source := testcatalog.New(testcatalog.Fixture())
	output := t.TempDir()
	website := views.Site{Name: "図書館", Origin: "https://library.example.test"}
	h := site.New(source, website, output)
	bookURL := views.BookURL(testcatalog.ID(1))
	w := request(t, h, bookURL, false, 200)
	for _, wanted := range []string{"日本語 &lt;script&gt; &amp; 本", "copy-" + testcatalog.ID(10), "100001", "貸出可", "貸出規則", "/author/" + testcatalog.ID(100) + ".html", "/publisher/" + testcatalog.ID(200) + ".html", "rel=\"canonical\"", "https://library.example.test" + bookURL, "scope=\"row\""} {
		if !strings.Contains(w.Body.String(), wanted) {
			t.Errorf("missing semantic detail %s", wanted)
		}
	}
	if strings.Contains(w.Body.String(), `href="javascript:`) || strings.Contains(w.Body.String(), "<script>alert") {
		t.Fatal("unsafe markup")
	}
	request(t, h, views.BookURL(testcatalog.ID(2)), false, 200)
	request(t, h, views.BookURL(testcatalog.ID(99)), false, 404)
	request(t, h, "/book/not-an-id.html", false, 404)
	request(t, h, "/author/"+testcatalog.ID(100)+".html", false, 200)
	request(t, h, "/publisher/"+testcatalog.ID(200)+".html", false, 200)
	request(t, h, "/author/"+testcatalog.ID(100)+"/2.html", false, 404)
	w = request(t, h, views.CopyURL("日本語 /?#{x}"), false, 302)
	if w.Header().Get("Location") != bookURL+"#copy-"+testcatalog.ID(11) {
		t.Fatalf("bad copy redirect: %s", w.Header().Get("Location"))
	}
	w = request(t, h, views.JSONURL(testcatalog.ID(1)), false, 200)
	var jsonFields map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &jsonFields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"copies", "createdAt", "createdBy", "updatedBy", "updatedAt", "version"} {
		if _, ok := jsonFields[field]; ok {
			t.Fatalf("citation exposes %s", field)
		}
	}
	builder := ssg.Builder{Source: source, Site: website, Output: output}
	if _, err := builder.Generate(context.Background()); err != nil {
		t.Fatal(err)
	}
	source.Fail(catalog.ErrUnavailable)
	// Static data survives upstream outages; dynamic search still reports the outage.
	request(t, h, bookURL, false, 200)
	request(t, h, "/", false, 200)
	request(t, h, views.JSONURL(testcatalog.ID(1)), false, 200)
	request(t, h, views.BookURL(testcatalog.ID(99)), false, 503)
	request(t, h, "/search", false, 503)
	w = request(t, h, "/ajax/search", true, 503)
	if !strings.Contains(w.Body.String(), "ただいま利用できません") || strings.Contains(w.Body.String(), "<!doctype") {
		t.Fatal("missing fragment error")
	}
}
func TestSearchProgressiveEnhancementAndValidation(t *testing.T) {
	data := testcatalog.Fixture()
	for i := 3; i <= 25; i++ {
		b := data.Books[1]
		b.ID = testcatalog.ID(i)
		data.Books = append(data.Books, b)
	}
	h := site.New(testcatalog.New(data), views.Site{Name: "図書館", Origin: "http://localhost:8082"}, t.TempDir())
	w := request(t, h, "/search?sort=title", false, 200)
	for _, wanted := range []string{`action="/search"`, `method="get"`, `hx-get="/ajax/search"`, `id="search-results"`, `rel="next"`, `href="/search?`} {
		if !strings.Contains(w.Body.String(), wanted) {
			t.Errorf("missing search behavior %s", wanted)
		}
	}
	w = request(t, h, "/ajax/search?page=2&sort=title", true, 200)
	if strings.Contains(strings.ToLower(w.Body.String()), "<!doctype") || w.Header().Get("HX-Push-Url") != "/search?page=2&sort=title" || w.Header().Get("Vary") != "HX-Request" {
		t.Fatal("fragment/history contract")
	}
	w = request(t, h, "/ajax/search?q=test", false, 303)
	if !strings.HasPrefix(w.Header().Get("Location"), "/search?") {
		t.Fatal("fragment deep link not redirected")
	}
	request(t, h, "/search?authorId="+testcatalog.ID(100), false, 200)
	request(t, h, "/search?publisherId="+testcatalog.ID(200), false, 200)
	for _, q := range []string{"page=0", "page=-1", "page=wat", "page=100001", "sort=oops", "authorId=no", "publisherId=no", "q=a&q=b", "q=" + strings.Repeat("x", 1001)} {
		request(t, h, "/search?"+q, false, 400)
	}
	w = request(t, h, "/search?q=見つからない", false, 200)
	if !strings.Contains(w.Body.String(), "該当する書誌情報はありません") {
		t.Fatal("zero results")
	}
	request(t, h, "/not-found", false, 404)
	w = request(t, h, views.Asset("htmx.min.js"), false, 200)
	if !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("unversioned asset caching")
	}
	request(t, h, "/assets/invalid/style.css", false, 404)
}

func TestAuthorityCanonicalURLCopyAndMissingReferences(t *testing.T) {
	data := testcatalog.Fixture()
	for i := 3; i <= 54; i++ {
		book := data.Books[0]
		book.ID = testcatalog.ID(1000 + i)
		book.Copies = []catalog.Copy{}
		data.Books = append(data.Books, book)
	}
	source := testcatalog.New(data)
	website := views.Site{Name: "図書館", Origin: "https://library.example.test"}
	h := site.New(source, website, t.TempDir())
	for _, path := range []string{views.AuthorityURL("author", testcatalog.ID(100), 1), views.AuthorityURL("author", testcatalog.ID(100), 2), views.AuthorityURL("publisher", testcatalog.ID(200), 2)} {
		w := request(t, h, path+"?tracking=ignored", false, 200)
		if !strings.Contains(w.Body.String(), website.Canonical(path)+"</textarea>") || !strings.Contains(w.Body.String(), "URL をコピー") {
			t.Fatalf("missing canonical URL copy: %s", path)
		}
		_, body, _ := strings.Cut(w.Body.String(), "<body>")
		if strings.Contains(body, "参照情報") || strings.Contains(body, "—") {
			t.Fatal("empty authority reference data rendered")
		}
	}
}

func TestRecordDatesCitationAndAssetsAcrossStaticAndDynamicPages(t *testing.T) {
	data := testcatalog.Fixture()
	created := time.Date(2025, 1, 1, 18, 2, 3, 0, time.UTC)
	updated := created.Add(24 * time.Hour)
	data.Books[0].CreatedAt = &created
	data.Books[0].UpdatedAt = updated
	data.Books[0].NDC = "913.6"
	data.Books[0].Reference["NDLBibID"] = "000012345"
	source := testcatalog.New(data)
	output := t.TempDir()
	website := views.Site{Name: "図書館", Origin: "https://library.example.test"}
	h := site.New(source, website, output)
	path := views.BookURL(data.Books[0].ID)
	dynamic := request(t, h, path, false, 200).Body.String()
	for _, want := range []string{views.TimestampValue(created), views.TimestampValue(updated), "2025-01-02 03:02:03 JST", "このページの生成日時", "http://id.ndl.go.jp/bib/000012345"} {
		if !strings.Contains(dynamic, want) {
			t.Errorf("missing timestamp/reference %s", want)
		}
	}
	for _, name := range []string{"book.svg", "pen.svg", "building.svg", "calendar.svg", "rulers.svg", "piggy-bank.svg", "sort-down.svg", "external.svg", "copy.js"} {
		if !strings.Contains(dynamic, views.Asset(name)) {
			t.Errorf("missing asset %s", name)
		}
		response := request(t, h, views.Asset(name), false, 200)
		if !strings.Contains(response.Header().Get("Cache-Control"), "immutable") {
			t.Error("new asset not cached by version")
		}
	}
	builder := ssg.Builder{Source: source, Site: website, Output: output}
	generation, err := builder.Generate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	source.Fail(catalog.ErrUnavailable)
	static := request(t, h, path, false, 200).Body.String()
	if !strings.Contains(static, views.TimestampValue(generation.GeneratedAt)) {
		t.Fatal("static page does not use generation timestamp")
	}
	citation := regexp.MustCompile(`(?s)<textarea id="citation-text"[^>]*>(.*?)</textarea>`)
	dynamicCitation, staticCitation := citation.FindStringSubmatch(dynamic), citation.FindStringSubmatch(static)
	if len(dynamicCitation) != 2 || len(staticCitation) != 2 || dynamicCitation[1] != staticCitation[1] {
		t.Fatal("static and dynamic citations differ")
	}
	if strings.Contains(static, "0001-01-01") {
		t.Fatal("zero date rendered")
	}
	// This fixture models an older API response without optional timestamps.
	old := request(t, h, views.BookURL(data.Books[1].ID), false, 200).Body.String()
	if strings.Contains(old, "書誌情報作成日時") || strings.Contains(old, "書誌情報最終更新日時") {
		t.Fatal("missing timestamps rendered")
	}
}
