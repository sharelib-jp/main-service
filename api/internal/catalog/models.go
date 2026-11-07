// Package catalog mirrors repository-service's version 1 public catalog contract.
// Keep these DTOs independent of storage models: actor fields and source payloads
// must never become public just because the database gains another column.
package catalog

import "time"

type AlternativeTitle struct {
	Title         string `json:"title"`
	Transcription string `json:"transcription"`
	Language      string `json:"language"`
}
type Authority struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"`
	Title             string             `json:"title"`
	Transcription     string             `json:"transcription"`
	Language          string             `json:"language"`
	AlternativeTitles []AlternativeTitle `json:"alternativeTitles"`
	BirthDate         string             `json:"birthDate,omitempty"`
	FoundationDate    string             `json:"foundationDate,omitempty"`
	References        map[string]any     `json:"references"`
}
type Author struct {
	Authority Authority `json:"authority"`
	Role      string    `json:"role"`
	Position  int       `json:"position"`
}
type Bibliography struct {
	ID                            string         `json:"id"`
	Title                         string         `json:"title"`
	TitleTranscription            string         `json:"titleTranscription"`
	Volume                        string         `json:"volume"`
	AuthorSummary                 string         `json:"authorSummary"`
	AuthorSummaryTranscription    string         `json:"authorSummaryTranscription"`
	PublisherSummary              string         `json:"publisherSummary"`
	PublisherSummaryTranscription string         `json:"publisherSummaryTranscription"`
	Authors                       []Author       `json:"authors"`
	Publisher                     *Authority     `json:"publisher,omitempty"`
	FirstPublishDate              string         `json:"firstPublishDate"`
	FirstPublishDatePrecision     string         `json:"firstPublishDatePrecision"`
	NDC                           string         `json:"ndc"`
	NDCEdition                    string         `json:"ndcEdition"`
	Language                      string         `json:"language"`
	Reference                     map[string]any `json:"reference"`
	OtherAttributes               map[string]any `json:"otherAttributes"`
}
type Copy struct {
	ID             string `json:"id"`
	ManagementCode string `json:"managementCode"`
	Condition      string `json:"condition"`
	LoanPolicy     string `json:"loanPolicy"`
}
type Book struct {
	Bibliography
	Copies    []Copy     `json:"copies"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}
type Snapshot struct {
	SchemaVersion int         `json:"schemaVersion"`
	GeneratedAt   time.Time   `json:"generatedAt"`
	Books         []Book      `json:"books"`
	Authors       []Authority `json:"authors"`
	Publishers    []Authority `json:"publishers"`
}
type Page struct {
	Items    []Book `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
}
type Location struct {
	BookID string `json:"bookId"`
	CopyID string `json:"copyId"`
}
