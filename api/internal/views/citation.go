package views

import (
	"strings"
	"time"

	"github.com/sharelib-jp/main-service/internal/catalog"
)

// Citation is shared by static and dynamic pages. It describes the stored
// bibliography and always ends at this library's canonical bibliographic URL.
func Citation(s Site, b catalog.Bibliography) string {
	parts := []string{}
	author := oneLine(b.AuthorSummary)
	if author == "" {
		names := []string{}
		for _, a := range b.Authors {
			name := oneLine(a.Authority.Title)
			if name == "" {
				continue
			}
			if role := oneLine(a.Role); role != "" {
				name += " [" + role + "]"
			}
			names = append(names, name)
		}
		author = strings.Join(names, "; ")
	}
	if author != "" {
		parts = append(parts, sentence(author))
	}
	title := oneLine(b.Title)
	if volume := oneLine(b.Volume); volume != "" {
		title = strings.TrimSpace(title + " " + volume)
	}
	publication := []string{}
	for _, value := range []string{title, oneLine(PublisherText(b)), PublicationDate(b.FirstPublishDate)} {
		if value != "" {
			publication = append(publication, value)
		}
	}
	if len(publication) > 0 {
		parts = append(parts, sentence(strings.Join(publication, ", ")))
	}
	for _, r := range References(b.Reference) {
		if strings.EqualFold(r.Key, "ISBN") && strings.TrimSpace(r.Text) != "" {
			parts = append(parts, sentence(oneLine(r.Text)))
			break
		}
	}
	parts = append(parts, s.Canonical(BookURL(b.ID)))
	return strings.Join(parts, " ")
}
func oneLine(value string) string { return strings.Join(strings.Fields(value), " ") }
func sentence(value string) string {
	// Stored responsibility statements may already include their terminal period.
	return strings.TrimRight(value, ".。 ") + "."
}
func PublicationDate(value string) string {
	value = strings.TrimSpace(value)
	for _, format := range []string{"2006-01-02", "2006-01", "2006"} {
		if t, err := time.Parse(format, value); err == nil {
			switch format {
			case "2006-01-02":
				return t.Format("2006.1.2")
			case "2006-01":
				return t.Format("2006.1")
			default:
				return t.Format("2006")
			}
		}
	}
	return value
}

var japanTime = time.FixedZone("JST", 9*60*60)

func Timestamp(t time.Time) string      { return t.In(japanTime).Format("2006-01-02 15:04:05 JST") }
func TimestampValue(t time.Time) string { return t.In(japanTime).Format(time.RFC3339Nano) }
