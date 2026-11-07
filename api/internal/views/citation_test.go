package views

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sharelib-jp/main-service/internal/catalog"
	"github.com/sharelib-jp/main-service/internal/testcatalog"
)

func TestCitationStoredFieldsAndMissingParts(t *testing.T) {
	site := Site{Name: "図書館", Origin: "https://library.example.test"}
	fixture := catalog.Bibliography{ID: testcatalog.ID(1), Title: "キノの旅 : the Beautiful World", Volume: "24", AuthorSummary: "時雨沢恵一 [作].", Publisher: &catalog.Authority{Title: "KADOKAWA"}, FirstPublishDate: "2025-09", Reference: map[string]any{"isbn": []any{"", "978-4-04-916590-6", "another ISBN"}, "NDLRecordURI": "https://ndlsearch.ndl.go.jp/books/external"}, OtherAttributes: map[string]any{"seriesTitle": "電撃文庫", "seriesNumber": "4359"}}
	want := "時雨沢恵一 [作]. キノの旅 : the Beautiful World 24, KADOKAWA, 2025.9. 978-4-04-916590-6. " + site.Canonical(BookURL(fixture.ID))
	if got := Citation(site, fixture); got != want {
		t.Fatalf("citation\ngot  %s\nwant %s", got, want)
	}
	tests := []struct {
		name string
		book catalog.Bibliography
		want string
	}{
		{"title only", catalog.Bibliography{ID: fixture.ID, Title: "題名"}, "題名. " + site.Canonical(BookURL(fixture.ID))},
		{"authors and roles", catalog.Bibliography{ID: fixture.ID, Title: "題名", Authors: []catalog.Author{{Authority: catalog.Authority{Title: "作者"}, Role: "作"}, {Authority: catalog.Authority{Title: "翻訳者"}, Role: "訳"}}, FirstPublishDate: "2025", Reference: map[string]any{"ISBN": "978-4-04-916590-6"}}, "作者 [作]; 翻訳者 [訳]. 題名, 2025. 978-4-04-916590-6. " + site.Canonical(BookURL(fixture.ID))},
		{"publisher summary", catalog.Bibliography{ID: fixture.ID, Title: "題名", PublisherSummary: "出版社", FirstPublishDate: "2025-09-01"}, "題名, 出版社, 2025.9.1. " + site.Canonical(BookURL(fixture.ID))},
		{"whitespace", catalog.Bibliography{ID: fixture.ID, Title: "題名\n改行", AuthorSummary: " \t", Volume: " ", PublisherSummary: " ", FirstPublishDate: " "}, "題名 改行. " + site.Canonical(BookURL(fixture.ID))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Citation(site, test.book); got != test.want {
				t.Errorf("got %q want %q", got, test.want)
			}
		})
	}
}
func TestPublicationPrecisionAndTimestamp(t *testing.T) {
	for _, test := range []struct{ input, want string }{{"2025", "2025"}, {"2025-09", "2025.9"}, {"2025-09-01", "2025.9.1"}, {"", ""}, {"保存された日付", "保存された日付"}} {
		if got := PublicationDate(test.input); got != test.want {
			t.Errorf("date %q => %q", test.input, got)
		}
	}
	utc := time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC)
	if got := Timestamp(utc); got != "2026-01-02 03:00:00 JST" {
		t.Errorf("display time: %s", got)
	}
	if got := TimestampValue(utc); got != "2026-01-02T03:00:00+09:00" {
		t.Errorf("machine time: %s", got)
	}
}
func TestMissingFieldsAndEscapingInPages(t *testing.T) {
	site := Site{Name: "図書館", Origin: "https://library.example.test"}
	b := testcatalog.Fixture().Books[1]
	b.Title = "</textarea><script>alert('x')</script>"
	var out bytes.Buffer
	if err := BookPage(site, b, time.Time{}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	_, html, _ := strings.Cut(out.String(), "<body>")
	for _, absent := range []string{"—", "id=\"summary-title\"", "id=\"references-title\"", "<th scope=\"row\">著者", "<th scope=\"row\">出版社", "record-footer", "<script>alert"} {
		if strings.Contains(html, absent) {
			t.Errorf("unexpected missing/unsafe field %q", absent)
		}
	}
	for _, present := range []string{"no-image.svg", "この書誌には蔵書が登録されていません。", "&lt;/textarea&gt;&lt;script&gt;", "data-copy-target=\"citation-text\"", "hidden"} {
		if !strings.Contains(html, present) {
			t.Errorf("missing content %q", present)
		}
	}

}
