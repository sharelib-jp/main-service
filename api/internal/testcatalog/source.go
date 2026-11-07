// Package testcatalog supplies an in-memory repository boundary for public-site tests.
package testcatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/sharelib-jp/main-service/internal/catalog"
)

func ID(n int) string { return fmt.Sprintf("%026d", n) }
func Fixture() catalog.Snapshot {
	a := catalog.Authority{ID: ID(100), Title: "著者の名前", Transcription: "チョシャ", Type: "Person", AlternativeTitles: []catalog.AlternativeTitle{{Title: "別名"}}, References: map[string]any{}}
	p := catalog.Authority{ID: ID(200), Title: "出版社", Type: "Organization", References: map[string]any{}}
	b := catalog.Book{Bibliography: catalog.Bibliography{ID: ID(1), Title: "日本語 <script> & 本", Volume: "第2巻", Authors: []catalog.Author{{Authority: a, Role: "著", Position: 1}}, Publisher: &p, FirstPublishDate: "2020-03", FirstPublishDatePrecision: "year_month", Reference: map[string]any{"ISBN": []any{"978-4-00-310101-8"}, "Link": "https://example.com/", "Unsafe": "javascript:alert(1)"}, OtherAttributes: map[string]any{"materialType": "図書", "extent": "100p", "price": "1000円", "description": "保存した要約"}}, Copies: []catalog.Copy{{ID: ID(10), ManagementCode: "100001", Condition: "normal", LoanPolicy: "available"}, {ID: ID(11), ManagementCode: "日本語 /?#{x}", Condition: "damaged", LoanPolicy: "in_library_only"}}}
	empty := catalog.Book{Bibliography: catalog.Bibliography{ID: ID(2), Title: "蔵書のない本", Authors: []catalog.Author{}, Reference: map[string]any{}, OtherAttributes: map[string]any{}}, Copies: []catalog.Copy{}}
	return catalog.Snapshot{SchemaVersion: 1, Books: []catalog.Book{b, empty}, Authors: []catalog.Authority{a}, Publishers: []catalog.Authority{p}}
}

type Source struct {
	mu      sync.RWMutex
	data    catalog.Snapshot
	failure error
}

func New(data catalog.Snapshot) *Source { s := &Source{}; s.Set(data); return s }
func clone(data catalog.Snapshot) catalog.Snapshot {
	raw, _ := json.Marshal(data)
	var v catalog.Snapshot
	_ = json.Unmarshal(raw, &v)
	return v
}
func (s *Source) Set(data catalog.Snapshot) { s.mu.Lock(); defer s.mu.Unlock(); s.data = clone(data) }
func (s *Source) Fail(err error)            { s.mu.Lock(); defer s.mu.Unlock(); s.failure = err }
func (s *Source) Snapshot(context.Context) (catalog.Snapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.data), s.failure
}
func (s *Source) Book(ctx context.Context, id string) (catalog.Book, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return catalog.Book{}, err
	}
	for _, b := range data.Books {
		if b.ID == id {
			return b, nil
		}
	}
	return catalog.Book{}, catalog.ErrNotFound
}
func (s *Source) Authority(ctx context.Context, kind, id string) (catalog.Authority, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return catalog.Authority{}, err
	}
	rows := data.Authors
	if kind == "publisher" {
		rows = data.Publishers
	}
	for _, a := range rows {
		if a.ID == id {
			return a, nil
		}
	}
	return catalog.Authority{}, catalog.ErrNotFound
}
func (s *Source) Lookup(ctx context.Context, code string) (catalog.Location, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return catalog.Location{}, err
	}
	for _, b := range data.Books {
		for _, c := range b.Copies {
			if c.ManagementCode == code {
				return catalog.Location{BookID: b.ID, CopyID: c.ID}, nil
			}
		}
	}
	return catalog.Location{}, catalog.ErrNotFound
}
func (s *Source) Search(ctx context.Context, o catalog.SearchOptions) (catalog.Page, error) {
	data, err := s.Snapshot(ctx)
	if err != nil {
		return catalog.Page{}, err
	}
	rows := []catalog.Book{}
	for _, b := range data.Books {
		if o.Query != "" && !strings.Contains(b.Title, o.Query) {
			continue
		}
		if o.PublisherID != "" && (b.Publisher == nil || b.Publisher.ID != o.PublisherID) {
			continue
		}
		if o.AuthorID != "" {
			found := false
			for _, a := range b.Authors {
				if a.Authority.ID == o.AuthorID {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		rows = append(rows, b)
	}
	if o.Sort == "title" {
		sort.Slice(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	}
	size := o.PageSize
	if size == 0 {
		size = 20
	}
	page := max(1, o.Page)
	start := min((page-1)*size, len(rows))
	end := min(start+size, len(rows))
	return catalog.Page{Items: rows[start:end], Total: int64(len(rows)), Page: page, PageSize: size}, nil
}
