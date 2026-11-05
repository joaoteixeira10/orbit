package uploads

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// What a file says, for search: Word, PowerPoint and Excel files (and
// OpenDocument ones) read here, from the XML inside; plain text as it is.
// A PDF's text comes from the app, which reads it with pdf.js (SetText).

// MaxText is the most text kept of one file.
const MaxText = 200_000

// TextOf reads what a file says ("" when it can't, or isn't one it reads).
func TextOf(name, typ string, data []byte) string { return clip(rawText(name, typ, data)) }

// Lines is what a file says with its lines kept — a text file as it is, a
// document a paragraph a line — at most MaxText.
func Lines(name, typ string, data []byte) string {
	s := rawText(name, typ, data)
	if len(s) > MaxText {
		s = s[:MaxText]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}

func rawText(name, typ string, data []byte) string {
	ext := strings.ToLower(path.Ext(name))
	switch ext {
	case ".docx", ".docm", ".dotx":
		return zipText(data, func(n string) bool {
			return n == "word/document.xml" || strings.HasPrefix(n, "word/header") || strings.HasPrefix(n, "word/footer")
		}, "t", "p")
	case ".pptx", ".pptm", ".ppsx":
		return zipText(data, func(n string) bool { return strings.HasPrefix(n, "ppt/slides/slide") && strings.HasSuffix(n, ".xml") }, "t", "p")
	case ".xlsx", ".xlsm":
		return zipText(data, func(n string) bool {
			return n == "xl/sharedStrings.xml" || (strings.HasPrefix(n, "xl/worksheets/sheet") && strings.HasSuffix(n, ".xml"))
		}, "t", "si")
	case ".odt", ".ods", ".odp":
		return zipText(data, func(n string) bool { return n == "content.xml" }, "", "p")
	case ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".xml", ".html", ".htm", ".log", ".yaml", ".yml", ".ini", ".rtf":
		if utf8.Valid(data) {
			return string(data)
		}
	}
	if strings.HasPrefix(typ, "text/") && utf8.Valid(data) {
		return string(data)
	}
	return ""
}

// zipText joins the character data of the XML parts chosen (in order of
// their names, so slide2 before slide10): with an element named word, its
// text only (Office's <w:t>, <a:t>, <t>; "" all text), a space after each
// one named para (a paragraph).
func zipText(data []byte, want func(string) bool, word, para string) string {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ""
	}
	var parts []*zip.File
	for _, f := range z.File {
		if want(f.Name) && f.UncompressedSize64 < 50<<20 {
			parts = append(parts, f)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return natural(parts[i].Name) < natural(parts[j].Name) })
	var b strings.Builder
	for _, f := range parts {
		r, err := f.Open()
		if err != nil {
			continue
		}
		d := xml.NewDecoder(io.LimitReader(r, 50<<20))
		in := 0
		for b.Len() < MaxText {
			tok, err := d.Token()
			if err != nil {
				break
			}
			switch t := tok.(type) {
			case xml.StartElement:
				if word == "" || t.Name.Local == word {
					in++
				}
			case xml.EndElement:
				if word == "" || t.Name.Local == word {
					in--
				}
				if t.Name.Local == para {
					b.WriteByte('\n')
				}
			case xml.CharData:
				if in > 0 {
					b.Write(t)
				}
			}
		}
		_ = r.Close()
		b.WriteByte('\n')
	}
	return b.String()
}

// natural pads the numbers in a name, so slide2 sorts before slide10.
func natural(s string) string {
	var b strings.Builder
	num := ""
	flush := func() {
		if num != "" {
			n, _ := strconv.Atoi(num)
			b.WriteString(strconv.FormatInt(int64(n)+1e9, 10))
			num = ""
		}
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			num += string(r)
			continue
		}
		flush()
		b.WriteRune(r)
	}
	flush()
	return b.String()
}

// clip squeezes runs of blank space and keeps at most MaxText.
func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > MaxText {
		s = s[:MaxText]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// SetText keeps what a file says (and reindexes it for search).
func SetText(db *gorm.DB, id, text string) error {
	text = clip(text)
	if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&models.UploadText{ID: id, Text: text}).Error; err != nil {
		return err
	}
	return db.Exec("UPDATE uploads SET id = id WHERE id = ?", id).Error // the search index follows the row
}

// HasText reports whether a file's text is kept.
func HasText(db *gorm.DB, id string) bool {
	var n int64
	db.Model(&models.UploadText{}).Where("id = ? AND text <> ''", id).Count(&n)
	return n > 0
}

// FillTexts reads the text of the files that have none yet and can be read
// here (once, at start: files from before search read them).
func FillTexts(db *gorm.DB) error {
	const done = "upload_texts_filled"
	var row models.Setting
	if db.Where(&models.Setting{Key: done}).Limit(1).Find(&row).RowsAffected == 1 {
		return nil
	}
	var files []models.Upload
	db.Select("id", "name", "type", "sha256").Where("id NOT IN (?)", db.Model(&models.UploadText{}).Select("id")).Find(&files)
	for _, u := range files {
		if !readable(u.Name, u.Type) {
			continue
		}
		data, err := Read(db, u)
		if err != nil {
			continue
		}
		if t := TextOf(u.Name, u.Type, data); t != "" {
			if err := SetText(db, u.ID, t); err != nil {
				return err
			}
		}
	}
	return db.Create(&models.Setting{Key: done, Value: "true"}).Error
}

// readable: a file TextOf may read something from.
func readable(name, typ string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".docx", ".docm", ".dotx", ".pptx", ".pptm", ".ppsx", ".xlsx", ".xlsm", ".odt", ".ods", ".odp", ".txt", ".md", ".markdown", ".csv", ".tsv", ".json", ".xml", ".html", ".htm", ".log", ".yaml", ".yml", ".ini", ".rtf":
		return true
	}
	return strings.HasPrefix(typ, "text/")
}
