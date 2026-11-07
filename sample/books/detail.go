package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/sharelib-jp/catalog-web-exporter/catalog"
)

var cat = `{"title":[{"value":"青春ブタ野郎はプチデビル後輩の夢を見ない","transcription":["セイシュン ブタ ヤロウ ワ プチ デビル コウハイ ノ ユメ オ ミナイ"]}],"author":[{"value":"鴨志田, 一","transcription":["カモシダ, ハジメ"]}],"publisher":[{"value":"KADOKAWA"}],"publication_date":"2014.8","ndc":{"NDC9":["913.6"]},"category":["図書"],"extent":["347p ; 15cm"],"price":"610円","reference_fields":{"ISBN":["978-4-04-866808-8"],"JPNO":["22453792"],"NDLBIBID":["025608104"],"TOHANMARCNO":["33132679"]},"publisher":[{"value":"KADOKAWA"}]}`

var desc = `タレント活動に復帰した麻衣先輩と、念願の彼氏彼女になり浮かれる咲太。──のはずが、翌朝起きたら付き合う前に戻っていた!?　これは僕がブタ野郎だからなのか？　青春のバカ野郎！　原因は“思春期症候群”だと考えた咲太は、事象を起こしている人物を探すことに。　そんな咲太の前に、尻を蹴り合った仲の後輩・朋絵が現れる。`

func unmarshalCat() (*catalog.BookCatalog, error) {
	var book catalog.BookCatalog
	err := json.Unmarshal([]byte(cat), &book)
	if err != nil {
		return nil, err
	}
	return &book, nil
}

func CvtArrayToString(arr []string, exc string) *string {
	ret := ""
	for _, s := range arr {
		cleaned := strings.ReplaceAll(s, exc, "")
		if cleaned != "" {
			return &cleaned
		}
		ret += cleaned + ""
	}
	// if ret != "" {
	// 	// Remove trailing comma
	// 	ret = strings.TrimSuffix(ret, ", ")
	// 	return &ret
	// }
	return nil
}

func normalizePersonName(s string) string {
	return strings.ReplaceAll(s, ", ", " ")
}

func JoinFieldValues(fields []catalog.EXTERNAL_FIELD) string {
	var parts []string
	for _, f := range fields {
		if f.VALUE != "" {
			parts = append(parts, normalizePersonName(f.VALUE))
		}
	}
	return strings.Join(parts, ", ")
}

func JoinFieldTranscriptions(fields []catalog.EXTERNAL_FIELD) string {
	var parts []string
	for _, f := range fields {
		if len(f.TRANSCRIPTION) > 0 {
			parts = append(parts, normalizePersonName(f.TRANSCRIPTION[0]))
		}
	}
	return strings.Join(parts, ", ")
}

func _main() {
	Catalog, _ := unmarshalCat()
	ID := uuid.New().String()
	props := map[string]any{"ID": ID, "Catalog": Catalog, "Desc": desc}
	fmt.Printf("%v", props)
	funcMap := template.FuncMap{
		"CvtArrayToString":        CvtArrayToString,
		"JoinFieldValues":         JoinFieldValues,
		"JoinFieldTranscriptions": JoinFieldTranscriptions,
	}
	t := template.Must(
		template.New("detail.html").Funcs(funcMap).ParseFiles("detail.html"),
	)
	f, err := os.Create("test.html")
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if err := t.Execute(f, props); err != nil {
		panic(err)
	}
}
