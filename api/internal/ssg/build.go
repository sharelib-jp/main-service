package ssg

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/views"
)

type Builder struct {
	Source         catalog.Source
	Site           views.Site
	Output         string
	GatewayManaged bool
}
type Generation struct {
	ID          string    `json:"id"`
	GeneratedAt time.Time `json:"generatedAt"`
	Books       int       `json:"books"`
	Authors     int       `json:"authors"`
	Publishers  int       `json:"publishers"`
}

func (b *Builder) Generate(ctx context.Context) (result Generation, err error) {
	started := time.Now()
	defer func() {
		if err != nil {
			log.Printf("catalog generation failed duration=%s error=%v", time.Since(started), err)
		} else {
			log.Printf("catalog generation ready id=%s books=%d authors=%d publishers=%d duration=%s", result.ID, result.Books, result.Authors, result.Publishers, time.Since(started))
		}
	}()
	output, err := filepath.Abs(b.Output)
	if err != nil {
		return result, err
	}
	guard, err := lock(output)
	if err != nil {
		return result, err
	}
	defer unlock(guard)
	snapshot, err := b.Source.Snapshot(ctx)
	if err != nil {
		return result, err
	}
	if snapshot.SchemaVersion != 1 {
		return result, fmt.Errorf("unsupported snapshot schema")
	}
	if err = validateSnapshot(snapshot); err != nil {
		return result, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return result, err
	}
	result = Generation{ID: "g-" + hex.EncodeToString(random[:]), GeneratedAt: time.Now().UTC(), Books: len(snapshot.Books), Authors: len(snapshot.Authors), Publishers: len(snapshot.Publishers)}
	dir, _ := generationDir(output, result.ID)
	stage := filepath.Join(output, "releases", ".build-"+result.ID)
	if err = os.MkdirAll(filepath.Join(stage, "public"), 0755); err != nil {
		return result, err
	}
	defer os.RemoveAll(stage)
	public := filepath.Join(stage, "public")
	write := func(path string, data []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return atomicWrite(filepath.Join(public, filepath.FromSlash(strings.TrimPrefix(path, "/"))), data, 0644)
	}
	render := func(path string, component templ.Component) error {
		var data bytes.Buffer
		if err := component.Render(ctx, &data); err != nil {
			return err
		}
		return write(path, data.Bytes())
	}
	if err = render("/index.html", views.Home(b.Site)); err != nil {
		return result, err
	}
	for _, book := range snapshot.Books {
		if err = render(views.BookURL(book.ID), views.BookPage(b.Site, book, result.GeneratedAt)); err != nil {
			return result, err
		}
		data, e := json.MarshalIndent(book.Bibliography, "", "  ")
		if e != nil {
			return result, e
		}
		if err = write(views.JSONURL(book.ID), append(data, '\n')); err != nil {
			return result, err
		}
	}
	byAuthor, byPublisher := map[string][]catalog.Book{}, map[string][]catalog.Book{}
	for _, book := range snapshot.Books {
		seen := map[string]bool{}
		for _, author := range book.Authors {
			if !seen[author.Authority.ID] {
				byAuthor[author.Authority.ID] = append(byAuthor[author.Authority.ID], book)
				seen[author.Authority.ID] = true
			}
		}
		if book.Publisher != nil {
			byPublisher[book.Publisher.ID] = append(byPublisher[book.Publisher.ID], book)
		}
	}
	authorities := func(kind string, rows []catalog.Authority, groups map[string][]catalog.Book) error {
		for _, a := range rows {
			books := groups[a.ID]
			sort.Slice(books, func(i, j int) bool { return books[i].ID < books[j].ID })
			pages := max(1, (len(books)+49)/50)
			for page := 1; page <= pages; page++ {
				start := min((page-1)*50, len(books))
				end := min(start+50, len(books))
				if err := render(views.AuthorityURL(kind, a.ID, page), views.AuthorityPage(b.Site, kind, a, books[start:end], page, pages, int64(len(books)))); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err = authorities("author", snapshot.Authors, byAuthor); err != nil {
		return result, err
	}
	if err = authorities("publisher", snapshot.Publishers, byPublisher); err != nil {
		return result, err
	}
	// Assets live outside generations so an open page keeps its versioned assets
	// after a newer catalog/template generation is published.
	if err = fs.WalkDir(views.Assets, ".", func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		data, e := fs.ReadFile(views.Assets, path)
		if e != nil {
			return e
		}
		return atomicWrite(filepath.Join(output, "assets", views.AssetVersion, path), data, 0644)
	}); err != nil {
		return result, err
	}
	if err = validateLinks(public, output); err != nil {
		return result, err
	}
	if err = atomicWrite(filepath.Join(stage, "routes.caddy"), []byte(routes(output, dir, snapshot.Books)), 0644); err != nil {
		return result, err
	}
	manifest, _ := json.MarshalIndent(result, "", "  ")
	if err = atomicWrite(filepath.Join(stage, "manifest.json"), manifest, 0644); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = os.Rename(stage, dir); err != nil {
		return result, err
	}
	if err = syncDirectory(filepath.Join(output, "releases")); err != nil {
		return result, err
	}
	if err = atomicWrite(filepath.Join(output, "control", "requested"), []byte(result.ID+"\n"), 0644); err != nil {
		return result, err
	}
	if !b.GatewayManaged {
		if err = publishID(output, result.ID); err != nil {
			return result, err
		}
		prune(output)
	}
	return result, nil
}
func validateSnapshot(s catalog.Snapshot) error {
	books, authors, publishers, copies, codes := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, group := range []struct {
		rows []catalog.Authority
		seen map[string]bool
	}{{s.Authors, authors}, {s.Publishers, publishers}} {
		for _, a := range group.rows {
			if !catalog.ValidID(a.ID) || group.seen[a.ID] || a.Title == "" {
				return fmt.Errorf("invalid or duplicate authority")
			}
			group.seen[a.ID] = true
		}
	}
	for _, b := range s.Books {
		if !catalog.ValidID(b.ID) || books[b.ID] || b.Title == "" {
			return fmt.Errorf("invalid or duplicate bibliography")
		}
		books[b.ID] = true
		for _, a := range b.Authors {
			if !authors[a.Authority.ID] {
				return fmt.Errorf("missing author authority for %s", b.ID)
			}
		}
		if b.Publisher != nil && !publishers[b.Publisher.ID] {
			return fmt.Errorf("missing publisher authority for %s", b.ID)
		}
		for _, c := range b.Copies {
			if !catalog.ValidID(c.ID) || copies[c.ID] || c.ManagementCode == "" || codes[c.ManagementCode] {
				return fmt.Errorf("invalid or duplicate copy for %s", b.ID)
			}
			copies[c.ID] = true
			codes[c.ManagementCode] = true
		}
	}
	return nil
}

var safeCode = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func routes(output, dir string, books []catalog.Book) string {
	var s strings.Builder
	s.WriteString("# Generated catalog generation. Imported inside a Caddy route block.\n")
	s.WriteString("map {path} {catalog_copy_destination} {\n")
	for _, b := range books {
		for _, c := range b.Copies {
			if safeCode.MatchString(c.ManagementCode) {
				fmt.Fprintf(&s, "    /copy/%s %s#copy-%s\n", c.ManagementCode, views.BookURL(b.ID), c.ID)
			}
		}
	}
	s.WriteString("    default \"\"\n}\n@catalog_copy expression `{catalog_copy_destination} != \"\"`\nheader @catalog_copy Cache-Control no-store\nredir @catalog_copy {catalog_copy_destination} 302\n")
	s.WriteString("header Cache-Control no-cache\nheader X-Content-Type-Options nosniff\n@catalog_assets path /assets/*\nheader @catalog_assets Cache-Control \"public, max-age=31536000, immutable\"\n")
	fmt.Fprintf(&s, "root @catalog_assets %s\nfile_server @catalog_assets {\n    pass_thru\n}\n", strconv.Quote(output))
	s.WriteString("header /resources/books/json/* Content-Type \"application/json; charset=utf-8\"\n")
	fmt.Fprintf(&s, "root * %s\nfile_server {\n    pass_thru\n}\n", strconv.Quote(filepath.Join(dir, "public")))
	return s.String()
}

var linkPattern = regexp.MustCompile(`(?:href|src)="([^"]+)"`)
var anchorPattern = regexp.MustCompile(`\bid="([^"]+)"`)

func validateLinks(public, output string) error {
	return filepath.WalkDir(public, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range linkPattern.FindAllSubmatch(data, -1) {
			u, err := url.Parse(html.UnescapeString(string(match[1])))
			if err != nil {
				return err
			}
			if u.IsAbs() || u.Host != "" {
				continue
			}
			if u.Path == "/search" || u.Path == "/ajax/search" || strings.HasPrefix(u.Path, "/copy/") {
				continue
			}
			target := path
			if u.Path != "" {
				if !strings.HasPrefix(u.Path, "/") {
					return fmt.Errorf("relative generated link in %s", path)
				}
				root := public
				if strings.HasPrefix(u.Path, "/assets/") {
					root = output
				}
				p := u.Path
				if p == "/" {
					p = "/index.html"
				}
				target = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(p, "/")))
			}
			content, err := os.ReadFile(target)
			if err != nil {
				return fmt.Errorf("broken link in %s: %s", path, u.Path)
			}
			if u.Fragment != "" {
				found := false
				for _, id := range anchorPattern.FindAllSubmatch(content, -1) {
					if html.UnescapeString(string(id[1])) == u.Fragment {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("broken anchor in %s", path)
				}
			}
		}
		return nil
	})
}
