package planner

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
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

var (
	fmHead   = regexp.MustCompile(`^---\n([\s\S]*?)\n---[ \t]*(?:\n|$)`)
	fmQuoted = regexp.MustCompile(`^("(?:[^"\\]|\\.)*")\s*:\s*(.*)$`)
	fmPlain  = regexp.MustCompile(`^([^:#\s][^:]*?)\s*:\s*(.*)$`)
	fmItem   = regexp.MustCompile(`^\s*-\s+`)
	fmNumber = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	fmDate   = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ReadFrontmatter takes a YAML header off Markdown text: its tags, and the
// rest as properties (simple "key: value" lines; lists only for tags) —
// what Frontmatter writes, and Obsidian and the like.
func ReadFrontmatter(md string) (tags []string, fields []Field, rest string) {
	m := fmHead.FindStringSubmatch(md)
	if m == nil {
		return nil, nil, md
	}
	unquote := func(s string) string { return strings.Trim(s, `"'`) }
	lines := strings.Split(m[1], "\n")
	for i := 0; i < len(lines); i++ {
		var key, raw string
		if q := fmQuoted.FindStringSubmatch(lines[i]); q != nil {
			if json.Unmarshal([]byte(q[1]), &key) != nil {
				continue
			}
			raw = q[2]
		} else if p := fmPlain.FindStringSubmatch(lines[i]); p != nil {
			key, raw = strings.TrimSpace(p[1]), p[2]
		} else {
			continue
		}
		raw = strings.TrimSpace(raw)
		if strings.EqualFold(key, "tags") {
			var list []any
			switch {
			case raw == "":
				for i+1 < len(lines) && fmItem.MatchString(lines[i+1]) {
					i++
					tags = append(tags, unquote(fmItem.ReplaceAllString(lines[i], "")))
				}
			case json.Unmarshal([]byte(strings.ReplaceAll(raw, "'", `"`)), &list) == nil:
				for _, t := range list {
					tags = append(tags, fmt.Sprint(t))
				}
			default:
				for _, t := range strings.Split(strings.Trim(raw, "[]"), ",") {
					tags = append(tags, strings.TrimLeft(unquote(strings.TrimSpace(t)), "#"))
				}
			}
			continue
		}
		if len(raw) > 1 && strings.ContainsAny(raw[:1], `"'`) && raw[len(raw)-1] == raw[0] {
			raw = raw[1 : len(raw)-1]
		}
		f := Field{Key: key, Type: "text", Value: raw}
		switch {
		case raw == "":
			f.Value = nil
		case raw == "true" || raw == "false":
			f.Type, f.Value = "checkbox", raw == "true"
		case fmNumber.MatchString(raw):
			n, _ := strconv.ParseFloat(raw, 64)
			f.Type, f.Value = "number", n
		case fmDate.MatchString(raw):
			f.Type = "date"
		case strings.HasPrefix(raw, "http:") || strings.HasPrefix(raw, "https:") || strings.HasPrefix(raw, "mailto:"):
			f.Type = "url"
		}
		if !slices.ContainsFunc(fields, func(g Field) bool { return strings.EqualFold(g.Key, key) }) && len(fields) < maxFields {
			fields = append(fields, f)
		}
	}
	return tags, fields, md[len(m[0]):]
}
