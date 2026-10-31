package planner

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

// Any page's tags and properties (database rows have their database's own
// properties besides). Tags: short words, each once (whatever its case),
// without "#". Properties: a name, a kind (text, number, date, checkbox,
// link) and a value of that kind, in the page's order.

const (
	maxTags     = 30
	maxTagLen   = 40
	maxFields   = 40
	maxFieldKey = 60
	maxFieldVal = 2000
)

// FieldKinds are the kinds of a page's property.
var FieldKinds = []string{"text", "number", "date", "checkbox", "url"}

// Field is one of a page's properties.
type Field struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

// CleanTags tidies a page's tags: no "#", single spaces, each once.
func CleanTags(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.Join(strings.Fields(strings.TrimLeft(strings.TrimSpace(t), "#")), " ")
		if t == "" {
			continue
		}
		if utf8.RuneCountInString(t) > maxTagLen || strings.ContainsAny(t, ",\n") {
			return nil, fmt.Errorf("a tag is a word or a few (at most %d characters, no commas)", maxTagLen)
		}
		if k := strings.ToLower(t); !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	if len(out) > maxTags {
		return nil, fmt.Errorf("at most %d tags", maxTags)
	}
	return out, nil
}

// CleanFields checks a page's properties (JSON []Field) and returns them
// tidied as JSON ("" for none).
func CleanFields(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" || raw == "null" {
		return "", nil
	}
	var in []Field
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return "", errors.New("properties: a list of {key, type, value}")
	}
	if len(in) > maxFields {
		return "", fmt.Errorf("at most %d properties", maxFields)
	}
	seen := map[string]bool{}
	out := make([]Field, 0, len(in))
	for _, f := range in {
		f.Key = strings.TrimSpace(f.Key)
		if f.Key == "" || utf8.RuneCountInString(f.Key) > maxFieldKey {
			return "", fmt.Errorf("a property needs a name (at most %d characters)", maxFieldKey)
		}
		if seen[strings.ToLower(f.Key)] {
			return "", fmt.Errorf("two properties named %q", f.Key)
		}
		seen[strings.ToLower(f.Key)] = true
		v, err := fieldValue(f.Type, f.Value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", f.Key, err)
		}
		f.Value = v
		out = append(out, f)
	}
	if len(out) == 0 {
		return "", nil
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

// fieldValue is a property's value made sure of its kind (nil: empty).
func fieldValue(kind string, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch kind {
	case "text":
		s, ok := v.(string)
		if !ok || utf8.RuneCountInString(s) > maxFieldVal {
			return nil, fmt.Errorf("text, at most %d characters", maxFieldVal)
		}
		return s, nil
	case "number":
		n, ok := v.(float64)
		if !ok {
			return nil, errors.New("a number")
		}
		return n, nil
	case "date":
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("a date (YYYY-MM-DD)")
		}
		if s == "" {
			return nil, nil
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, errors.New("a date (YYYY-MM-DD)")
		}
		return s, nil
	case "checkbox":
		b, ok := v.(bool)
		if !ok {
			return nil, errors.New("true or false")
		}
		return b, nil
	case "url":
		s, ok := v.(string)
		if !ok || len(s) > maxFieldVal {
			return nil, errors.New("a link")
		}
		if s == "" {
			return nil, nil
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "mailto") {
			return nil, errors.New("a link (http, https or mailto)")
		}
		return s, nil
	}
	return nil, fmt.Errorf("a kind of %s", strings.Join(FieldKinds, ", "))
}

// FieldsText is a page's properties as words (for search).
func FieldsText(raw string) string {
	var fs []Field
	_ = json.Unmarshal([]byte(raw), &fs)
	var b strings.Builder
	for _, f := range fs {
		b.WriteString(f.Key)
		b.WriteByte(' ')
		if f.Value != nil {
			fmt.Fprint(&b, f.Value)
		}
		b.WriteByte(' ')
	}
	return strings.TrimSpace(b.String())
}

// Frontmatter is a page's tags and properties as a Markdown file's YAML
// header ("" when it has none): Obsidian and the like read it.
func Frontmatter(tags, fields string) string {
	var ts []string
	_ = json.Unmarshal([]byte(tags), &ts)
	var fs []Field
	_ = json.Unmarshal([]byte(fields), &fs)
	if len(ts) == 0 && len(fs) == 0 {
		return ""
	}
	q := func(v any) string { b, _ := json.Marshal(v); return string(b) } // JSON is YAML
	var b strings.Builder
	b.WriteString("---\n")
	if len(ts) > 0 {
		b.WriteString("tags: " + q(ts) + "\n")
	}
	for _, f := range fs {
		key := f.Key
		if strings.ContainsAny(key, ":#\"'{}[],&*?|<>=!%@`") || strings.ToLower(key) == "tags" {
			key = q(key)
		}
		if f.Value == nil {
			b.WriteString(key + ":\n")
			continue
		}
		b.WriteString(key + ": " + q(f.Value) + "\n")
	}
	b.WriteString("---\n")
	return b.String()
}
