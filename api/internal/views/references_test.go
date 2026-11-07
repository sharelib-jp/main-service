package views

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReferenceLinksAndPreservedDisplay(t *testing.T) {
	tests := []struct{ key, value, want string }{
		{"NDLBibID", "034263545", "http://id.ndl.go.jp/bib/034263545"},
		{"ndlbibid", "000012345", "http://id.ndl.go.jp/bib/000012345"},
		{"JPNO", "00008614", "http://id.ndl.go.jp/jpno/00008614"},
		{"NDLAuthorityID", "00054222", "http://id.ndl.go.jp/auth/ndlna/00054222"},
		{"NCID", "BA1234567X", "https://ci.nii.ac.jp/ncid/BA1234567X"},
		{"Wikidata", "Q123", "https://www.wikidata.org/wiki/Q123"},
		{"VIAF", "12345", "https://viaf.org/viaf/12345/"},
		{"ISNI", "0000 0001 2124 423X", "https://isni.org/isni/000000012124423X"},
		{"ISBN", "978-4-04-916590-6", "https://ndlsearch.ndl.go.jp/search?cs=bib&keyword=9784049165906"},
		{"ISBN", "4-00-310101-4", "https://ndlsearch.ndl.go.jp/search?cs=bib&keyword=4003101014"},
		{"TOHANMARCNO", "0012345", "https://ndlsearch.ndl.go.jp/search?cs=bib&keyword=0012345"},
		{"NDLRecordURI", "https://ndlsearch.ndl.go.jp/books/R100000002-I034263545", "https://ndlsearch.ndl.go.jp/books/R100000002-I034263545"},
		{"Custom", "https://example.test/path?q=a&b=c", "https://example.test/path?q=a&b=c"},
		{"Unknown", "12345", ""},
		{"NDLBIBID", "../bad?query=x", ""},
		{"ISBN", "not an identifier", ""},
		{"VIAF", "123/evil", ""},
		{"Custom", "javascript:alert(1)", ""},
		{"Custom", "//example.test/path", ""},
		{"Custom", "https://user:secret@example.test/path", ""},
		{"Custom", "https:///missing-host", ""},
	}
	for _, test := range tests {
		t.Run(test.key+"/"+test.value, func(t *testing.T) {
			rows := References(map[string]any{test.key: test.value})
			if len(rows) != 1 || rows[0].Text != test.value || rows[0].URL != test.want {
				t.Fatalf("got %+v want URL %q", rows, test.want)
			}
		})
	}
	rows := References(map[string]any{"NDLBIBID": []any{"", "000012345", "034263545", nil}, "empty": []string{}, "whitespace": " \n", "nil": nil, "object": map[string]any{}, "number": json.Number("9007199254740993")})
	if len(rows) != 3 || rows[0].Text != "000012345" || rows[1].Text != "034263545" || rows[2].Text != "9007199254740993" {
		t.Fatalf("array/number preservation: %+v", rows)
	}
	if rows[0].URL == "" || strings.Contains(rows[0].URL, "12345/") {
		t.Fatal("identifier altered")
	}
	if rows := References(map[string]any{"JPNO": []string{"00008614", "12345678"}}); len(rows) != 2 {
		t.Fatal("typed string arrays not supported")
	}
}
