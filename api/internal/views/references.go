package views

import (
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type Reference struct{ Key, Text, URL string }

var numericIdentifier = regexp.MustCompile(`^[0-9]+$`)
var ncidIdentifier = regexp.MustCompile(`^[A-Z]{2}[0-9]{7}[0-9X]$`)
var wikidataIdentifier = regexp.MustCompile(`^Q[1-9][0-9]*$`)
var isniIdentifier = regexp.MustCompile(`^[0-9]{15}[0-9X]$`)
var isbnIdentifier = regexp.MustCompile(`^(?:[0-9]{13}|[0-9]{9}[0-9X])$`)

func referenceURL(key, value string) string {
	value = strings.TrimSpace(value)
	// Explicit HTTP(S) values continue to work for custom reference fields.
	if u, err := url.Parse(value); err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil {
		return u.String()
	}
	switch strings.ToUpper(key) {
	case "NDLBIBID":
		if numericIdentifier.MatchString(value) {
			return "http://id.ndl.go.jp/bib/" + value
		}
	case "JPNO":
		if numericIdentifier.MatchString(value) {
			return "http://id.ndl.go.jp/jpno/" + value
		}
	case "NDLAUTHORITYID":
		if numericIdentifier.MatchString(value) {
			return "http://id.ndl.go.jp/auth/ndlna/" + value
		}
	case "NCID":
		id := strings.ToUpper(value)
		if ncidIdentifier.MatchString(id) {
			return "https://ci.nii.ac.jp/ncid/" + id
		}
	case "WIKIDATA":
		id := strings.ToUpper(value)
		if wikidataIdentifier.MatchString(id) {
			return "https://www.wikidata.org/wiki/" + id
		}
	case "VIAF":
		if numericIdentifier.MatchString(value) {
			return "https://viaf.org/viaf/" + value + "/"
		}
	case "ISNI":
		id := strings.ToUpper(strings.ReplaceAll(value, " ", ""))
		if isniIdentifier.MatchString(id) {
			return "https://isni.org/isni/" + id
		}
	case "ISBN":
		id := strings.ToUpper(strings.NewReplacer("-", "", "‐", "", "−", "", " ", "").Replace(value))
		if isbnIdentifier.MatchString(id) {
			return ndlSearchURL(id)
		}
	case "TOHANMARCNO":
		if numericIdentifier.MatchString(value) {
			return ndlSearchURL(value)
		}
	}
	return ""
}
func ndlSearchURL(value string) string {
	return "https://ndlsearch.ndl.go.jp/search?" + url.Values{"cs": {"bib"}, "keyword": {value}}.Encode()
}
func References(values map[string]any) []Reference {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []Reference{}
	appendText := func(key, text string) {
		if strings.TrimSpace(text) != "" {
			out = append(out, Reference{Key: key, Text: text, URL: referenceURL(key, text)})
		}
	}
	var add func(string, any)
	add = func(key string, value any) {
		switch x := value.(type) {
		case nil:
			return
		case []any:
			for _, item := range x {
				add(key, item)
			}
		case []string:
			for _, item := range x {
				add(key, item)
			}
		case string:
			appendText(key, x)
		case map[string]any:
			if len(x) > 0 {
				data, _ := json.Marshal(x)
				appendText(key, string(data))
			}
		default:
			if data, err := json.Marshal(value); err == nil {
				appendText(key, string(data))
			}
		}
	}
	for _, key := range keys {
		add(key, values[key])
	}
	return out
}
