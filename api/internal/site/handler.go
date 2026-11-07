// Package site serves public semantic HTML, enhancing ordinary navigation with HTMX.
package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/ssg"
	"github.com/sharelib-jp/main-service/internal/views"
)

type Handler struct {
	Source catalog.Source
	Site   views.Site
	Output string
	mux    *http.ServeMux
}

func New(source catalog.Source, site views.Site, output string) *Handler {
	h := &Handler{Source: source, Site: site, Output: output, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /{$}", h.home)
	h.mux.HandleFunc("GET /search", h.search)
	h.mux.HandleFunc("GET /ajax/search", h.search)
	h.mux.HandleFunc("GET /book/{name}", h.book)
	h.mux.HandleFunc("GET /author/{rest...}", func(w http.ResponseWriter, r *http.Request) { h.authority(w, r, "author") })
	h.mux.HandleFunc("GET /publisher/{rest...}", func(w http.ResponseWriter, r *http.Request) { h.authority(w, r, "publisher") })
	h.mux.HandleFunc("GET /copy/{code...}", h.copy)
	h.mux.HandleFunc("GET /resources/books/json/{id}", h.citation)
	h.mux.HandleFunc("GET /assets/{version}/{name}", h.asset)
	h.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { h.error(w, r, catalog.ErrNotFound, false) })
	return h
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	h.mux.ServeHTTP(w, r)
}
func (h *Handler) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render HTML: %v", err)
	}
}
func (h *Handler) error(w http.ResponseWriter, r *http.Request, err error, fragment bool) {
	status, title, message := 503, "ただいま利用できません", "書誌情報を取得できませんでした。時間をおいて再度お試しください。"
	if errors.Is(err, catalog.ErrNotFound) {
		status, title, message = 404, "資料が見つかりません", "指定した書誌・典拠・蔵書は見つかりませんでした。"
	} else if errors.Is(err, catalog.ErrInvalid) {
		status, title, message = 400, "検索条件を確認してください", "検索語、典拠 ID、並び順、ページ番号を確認してください。"
	}
	if fragment {
		h.render(w, r, status, views.ErrorContent(title, message))
	} else {
		h.render(w, r, status, views.ErrorPage(h.Site, r.URL.RequestURI(), title, message))
	}
}
func (h *Handler) static(w http.ResponseWriter, r *http.Request, path string) bool {
	id := ssg.Current(h.Output)
	if id == "" {
		return false
	}
	root := ssg.PublicDir(h.Output, id)
	if root == "" {
		return false
	}
	if path == "/" {
		path = "/index.html"
	}
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path, "/"))))
	if err != nil {
		return false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if strings.HasPrefix(path, "/resources/books/json/") {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	return true
}
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	if !h.static(w, r, "/") {
		h.render(w, r, 200, views.Home(h.Site))
	}
}
func (h *Handler) book(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !strings.HasSuffix(name, ".html") {
		h.error(w, r, catalog.ErrNotFound, false)
		return
	}
	id := strings.TrimSuffix(name, ".html")
	if !catalog.ValidID(id) {
		h.error(w, r, catalog.ErrNotFound, false)
		return
	}
	if h.static(w, r, views.BookURL(id)) {
		return
	}
	book, err := h.Source.Book(r.Context(), id)
	if err != nil {
		h.error(w, r, err, false)
		return
	}
	h.render(w, r, 200, views.BookPage(h.Site, book, time.Now().UTC()))
}
func (h *Handler) authority(w http.ResponseWriter, r *http.Request, kind string) {
	parts := strings.Split(r.PathValue("rest"), "/")
	page := 1
	var id string
	if len(parts) == 1 && strings.HasSuffix(parts[0], ".html") {
		id = strings.TrimSuffix(parts[0], ".html")
	} else if len(parts) == 2 && strings.HasSuffix(parts[1], ".html") {
		id = parts[0]
		var err error
		page, err = strconv.Atoi(strings.TrimSuffix(parts[1], ".html"))
		if err != nil || page < 1 || page > 100000 {
			h.error(w, r, catalog.ErrNotFound, false)
			return
		}
	} else {
		h.error(w, r, catalog.ErrNotFound, false)
		return
	}
	if !catalog.ValidID(id) {
		h.error(w, r, catalog.ErrNotFound, false)
		return
	}
	if len(parts) == 2 && page == 1 {
		http.Redirect(w, r, views.AuthorityURL(kind, id, 1), 302)
		return
	}
	if h.static(w, r, views.AuthorityURL(kind, id, page)) {
		return
	}
	a, err := h.Source.Authority(r.Context(), kind, id)
	if err != nil {
		h.error(w, r, err, false)
		return
	}
	o := catalog.SearchOptions{Page: page, PageSize: 50}
	if kind == "author" {
		o.AuthorID = id
	} else {
		o.PublisherID = id
	}
	result, err := h.Source.Search(r.Context(), o)
	if err != nil {
		h.error(w, r, err, false)
		return
	}
	pages := max(1, int((result.Total+49)/50))
	if page > pages {
		h.error(w, r, catalog.ErrNotFound, false)
		return
	}
	h.render(w, r, 200, views.AuthorityPage(h.Site, kind, a, result.Items, page, pages, result.Total))
}
func (h *Handler) copy(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if code == "" || len(code) > 256 {
		h.error(w, r, catalog.ErrNotFound, false)
		return
	}
	loc, err := h.Source.Lookup(r.Context(), code)
	if err != nil {
		h.error(w, r, err, false)
		return
	}
	if !catalog.ValidID(loc.BookID) || !catalog.ValidID(loc.CopyID) {
		h.error(w, r, catalog.ErrUnavailable, false)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, views.BookURL(loc.BookID)+"#copy-"+loc.CopyID, 302)
}
func (h *Handler) citation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	jsonError := func(status int, message string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
	}
	if !catalog.ValidID(id) {
		jsonError(404, "書誌情報が見つかりません。")
		return
	}
	if h.static(w, r, views.JSONURL(id)) {
		return
	}
	book, err := h.Source.Book(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrNotFound) {
			jsonError(404, "書誌情報が見つかりません。")
		} else {
			jsonError(503, "書誌情報を取得できません。時間をおいて再度お試しください。")
		}
		return
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(book.Bibliography); err != nil {
		log.Printf("encode citation: %v", err)
	}
}
func parseSearch(r *http.Request) (catalog.SearchOptions, error) {
	q := r.URL.Query()
	o := catalog.SearchOptions{Query: strings.TrimSpace(q.Get("q")), AuthorID: q.Get("authorId"), PublisherID: q.Get("publisherId"), Sort: q.Get("sort"), Page: 1, PageSize: 20}
	for _, name := range []string{"q", "authorId", "publisherId", "sort", "page"} {
		if len(q[name]) > 1 {
			return o, catalog.ErrInvalid
		}
	}
	if len(o.Query) > 1000 || (o.AuthorID != "" && !catalog.ValidID(o.AuthorID)) || (o.PublisherID != "" && !catalog.ValidID(o.PublisherID)) {
		return o, catalog.ErrInvalid
	}
	if o.Sort == "" {
		o.Sort = "updated"
	}
	if o.Sort != "updated" && o.Sort != "title" && o.Sort != "published" {
		return o, catalog.ErrInvalid
	}
	if p := q.Get("page"); p != "" {
		var err error
		o.Page, err = strconv.Atoi(p)
		if err != nil || o.Page < 1 || o.Page > 100000 {
			return o, catalog.ErrInvalid
		}
	}
	return o, nil
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	ajax := r.URL.Path == "/ajax/search"
	if ajax {
		w.Header().Set("Vary", "HX-Request")
	}
	fragment := ajax && r.Header.Get("HX-Request") == "true"
	o, err := parseSearch(r)
	if err != nil {
		h.error(w, r, err, fragment)
		return
	}
	if ajax && !fragment {
		http.Redirect(w, r, views.SearchURL(o, o.Page), 303)
		return
	}
	result, err := h.Source.Search(r.Context(), o)
	if err != nil {
		h.error(w, r, err, fragment)
		return
	}
	v := views.SearchView{Options: o, Page: result}
	for _, filter := range []struct {
		kind, id string
		dest     *string
	}{{"author", o.AuthorID, &v.Author}, {"publisher", o.PublisherID, &v.Publisher}} {
		if filter.id != "" {
			a, e := h.Source.Authority(r.Context(), filter.kind, filter.id)
			if e != nil {
				h.error(w, r, e, fragment)
				return
			}
			*filter.dest = a.Title
		}
	}
	if fragment {
		w.Header().Set("HX-Push-Url", views.SearchURL(o, o.Page))
		h.render(w, r, 200, views.SearchResults(v))
	} else {
		h.render(w, r, 200, views.SearchPage(h.Site, v))
	}
}
func (h *Handler) asset(w http.ResponseWriter, r *http.Request) {
	version, name := r.PathValue("version"), r.PathValue("name")
	if !fs.ValidPath(version) || !fs.ValidPath(name) || strings.ContainsAny(version+name, "/\\") || version == "." || name == "." {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	if file, err := os.Open(filepath.Join(h.Output, "assets", version, name)); err == nil {
		defer file.Close()
		if info, e := file.Stat(); e == nil && info.Mode().IsRegular() {
			http.ServeContent(w, r, name, info.ModTime(), file)
			return
		}
	}
	if version != views.AssetVersion {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(views.Assets, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", fmt.Sprintf(`"%s-%s"`, version, name))
	http.ServeContent(w, r, name, time.Time{}, strings.NewReader(string(data)))
}
