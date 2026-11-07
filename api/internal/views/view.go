package views

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/url"
	"strings"

	"github.com/sharelib-jp/main-service/internal/catalog"
)

//go:generate go run github.com/a-h/templ/cmd/templ@v0.3.1100 generate
//go:embed assets/*
var assetFiles embed.FS
var Assets, _ = fs.Sub(assetFiles, "assets")
var AssetVersion = assetVersion()

func assetVersion() string {
	h := sha256.New()
	names, _ := fs.ReadDir(Assets, ".")
	for _, n := range names {
		data, _ := fs.ReadFile(Assets, n.Name())
		h.Write([]byte(n.Name()))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
func Asset(name string) string { return "/assets/" + AssetVersion + "/" + name }

type Site struct{ Name, Origin string }

func (s Site) Canonical(path string) string { return strings.TrimRight(s.Origin, "/") + path }
func BookURL(id string) string              { return "/book/" + id + ".html" }
func AuthorityURL(kind, id string, page int) string {
	if page <= 1 {
		return "/" + kind + "/" + id + ".html"
	}
	return fmt.Sprintf("/%s/%s/%d.html", kind, id, page)
}
func JSONURL(id string) string   { return "/resources/books/json/" + id }
func CopyURL(code string) string { return "/copy/" + url.PathEscape(code) }
func SearchURL(o catalog.SearchOptions, page int) string {
	o.Page = page
	return "/search?" + o.Values().Encode()
}
func Other(b catalog.Bibliography, key string) string {
	v, _ := b.OtherAttributes[key].(string)
	return strings.TrimSpace(v)
}
func AuthorText(b catalog.Bibliography) string {
	if strings.TrimSpace(b.AuthorSummary) != "" {
		return strings.TrimSpace(b.AuthorSummary)
	}
	s := []string{}
	for _, a := range b.Authors {
		if title := strings.TrimSpace(a.Authority.Title); title != "" {
			s = append(s, title)
		}
	}
	return strings.Join(s, "、")
}
func PublisherText(b catalog.Bibliography) string {
	if b.Publisher != nil && strings.TrimSpace(b.Publisher.Title) != "" {
		return strings.TrimSpace(b.Publisher.Title)
	}
	return strings.TrimSpace(b.PublisherSummary)
}
func Condition(s string) string {
	v := map[string]string{"normal": "通常", "lost": "紛失", "damaged": "破損", "stolen": "盗難", "unknown": "不明"}
	return v[s]
}
func LoanPolicy(s string) string {
	v := map[string]string{"available": "貸出可", "suspended": "貸出停止", "no_loan": "貸出不可", "in_library_only": "館内閲覧のみ", "no_access": "利用不可"}
	return v[s]
}

type SearchView struct {
	Options           catalog.SearchOptions
	Page              catalog.Page
	Author, Publisher string
}

func (v SearchView) Pages() int {
	return int((v.Page.Total + int64(v.Page.PageSize) - 1) / int64(max(1, v.Page.PageSize)))
}

type SummaryField struct{ Label, Value, Icon string }

func Summary(b catalog.Bibliography) []SummaryField {
	fields := []SummaryField{
		{"資料種別", Other(b, "materialType"), "book.svg"},
		{"著者", AuthorText(b), "pen.svg"},
		{"出版社", PublisherText(b), "building.svg"},
		{"出版年月日", b.FirstPublishDate, "calendar.svg"},
		{"形態", Other(b, "extent"), "rulers.svg"},
		{"価格", Other(b, "price"), "piggy-bank.svg"},
		{"NDC", b.NDC, "sort-down.svg"},
	}
	out := []SummaryField{}
	for _, field := range fields {
		if strings.TrimSpace(field.Value) != "" {
			out = append(out, field)
		}
	}
	return out
}
func BookMetadata(b catalog.Book) string {
	parts := []string{}
	for _, value := range []string{PublisherText(b.Bibliography), strings.TrimSpace(b.FirstPublishDate)} {
		if value != "" {
			parts = append(parts, value)
		}
	}
	parts = append(parts, fmt.Sprintf("蔵書 %d冊", len(b.Copies)))
	return strings.Join(parts, " · ")
}

func HasText(value string) bool { return strings.TrimSpace(value) != "" }
