package planner

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Databases: a page can be a database — a table of the pages inside it,
// each with values for its properties. The page holds the properties and
// the saved views (DB); each row page holds its values (Props), keyed by
// property id.

// Property types.
const (
	PropText     = "text"
	PropNumber   = "number"
	PropSelect   = "select"
	PropMulti    = "multi_select"
	PropDate     = "date"
	PropCheckbox = "checkbox"
	PropPerson   = "person"
	PropURL      = "url"
)

var propTypes = map[string]bool{PropText: true, PropNumber: true, PropSelect: true, PropMulti: true, PropDate: true, PropCheckbox: true, PropPerson: true, PropURL: true}

// DBOption is a choice of a select or multi-select property.
type DBOption struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// DBProp is a property (a column).
type DBProp struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	Options []DBOption `json:"options,omitempty"`
}

// DBFilter keeps the rows whose property Prop ("title" for the name)
// passes Op (with Value).
type DBFilter struct {
	Prop  string `json:"prop"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}

// DBSort orders the rows by a property.
type DBSort struct {
	Prop string `json:"prop"`
	Desc bool   `json:"desc,omitempty"`
}

// DBView is a saved way to look at the rows: a table or a board (grouped by
// a select property), filtered, sorted, some properties hidden.
type DBView struct {
	ID      string     `json:"id"`
	Name    string     `json:"name"`
	Type    string     `json:"type"` // table | board
	GroupBy string     `json:"group_by,omitempty"`
	Filters []DBFilter `json:"filters,omitempty"`
	Sorts   []DBSort   `json:"sorts,omitempty"`
	Hidden  []string   `json:"hidden,omitempty"`
}

// Database is what a database page holds.
type Database struct {
	Props []DBProp `json:"props"`
	Views []DBView `json:"views"`
}

const maxText = 2000

func short(s string, n int) bool { return len(s) <= n }

// CleanDatabase checks a database's JSON and gives it back tidy ("" when
// none: the page is no longer a database).
func CleanDatabase(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return "", nil
	}
	if len(raw) > 64<<10 {
		return "", errors.New("the database is too big")
	}
	var db Database
	if err := json.Unmarshal([]byte(raw), &db); err != nil {
		return "", errors.New("invalid database: " + err.Error())
	}
	if len(db.Props) > 50 || len(db.Views) > 20 {
		return "", errors.New("a database has at most 50 properties and 20 views")
	}
	ids := map[string]bool{"title": true}
	for i := range db.Props {
		p := &db.Props[i]
		p.Name = strings.TrimSpace(p.Name)
		if p.ID == "" || !short(p.ID, 40) || ids[p.ID] {
			return "", errors.New("each property needs its own id")
		}
		ids[p.ID] = true
		if p.Name == "" || !short(p.Name, 100) {
			return "", errors.New("each property needs a name (up to 100 characters)")
		}
		if !propTypes[p.Type] {
			return "", fmt.Errorf("unknown property type %q", p.Type)
		}
		if p.Type != PropSelect && p.Type != PropMulti {
			p.Options = nil
		}
		if len(p.Options) > 100 {
			return "", errors.New("a property has at most 100 options")
		}
		seen := map[string]bool{}
		for j := range p.Options {
			o := &p.Options[j]
			o.Name = strings.TrimSpace(o.Name)
			if o.ID == "" || seen[o.ID] || !short(o.ID, 40) || o.Name == "" || !short(o.Name, 100) || !short(o.Color, 9) {
				return "", errors.New("each option needs its own id and a name")
			}
			seen[o.ID] = true
		}
	}
	for i := range db.Views {
		v := &db.Views[i]
		v.Name = strings.TrimSpace(v.Name)
		if v.ID == "" || !short(v.ID, 40) || v.Name == "" || !short(v.Name, 100) {
			return "", errors.New("each view needs an id and a name")
		}
		if v.Type != "table" && v.Type != "board" {
			return "", errors.New("a view is a table or a board")
		}
		if len(v.Filters) > 20 || len(v.Sorts) > 5 || len(v.Hidden) > 50 {
			return "", errors.New("a view has at most 20 filters and 5 sorts")
		}
		for _, f := range v.Filters {
			if !ids[f.Prop] || !short(f.Op, 20) {
				return "", errors.New("a filter needs one of the properties")
			}
			if s, ok := f.Value.(string); ok && !short(s, maxText) {
				return "", errors.New("a filter's value is too long")
			}
		}
		for _, s := range v.Sorts {
			if !ids[s.Prop] {
				return "", errors.New("a sort needs one of the properties")
			}
		}
	}
	if db.Props == nil {
		db.Props = []DBProp{}
	}
	if db.Views == nil {
		db.Views = []DBView{}
	}
	b, _ := json.Marshal(db)
	return string(b), nil
}

// ParseDatabase reads a page's database (nil when it's none).
func ParseDatabase(raw string) *Database {
	if raw == "" {
		return nil
	}
	var db Database
	if json.Unmarshal([]byte(raw), &db) != nil {
		return nil
	}
	return &db
}

// MergeProps applies a patch of property values to a row's (null removes
// one) and gives back the new JSON.
func MergeProps(current, patch string) (string, error) {
	if len(patch) > 64<<10 {
		return "", errors.New("too many values")
	}
	var p map[string]json.RawMessage
	if err := json.Unmarshal([]byte(patch), &p); err != nil {
		return "", errors.New("props must be an object")
	}
	cur := map[string]json.RawMessage{}
	if current != "" {
		_ = json.Unmarshal([]byte(current), &cur)
	}
	for k, v := range p {
		if k == "" || !short(k, 40) {
			return "", errors.New("a property id is empty or too long")
		}
		if string(v) == "null" {
			delete(cur, k)
			continue
		}
		var x any
		if err := json.Unmarshal(v, &x); err != nil {
			return "", err
		}
		switch t := x.(type) {
		case string:
			if !short(t, maxText) {
				return "", errors.New("a value is too long")
			}
		case float64, bool:
		case []any:
			if len(t) > 50 {
				return "", errors.New("at most 50 choices")
			}
			for _, e := range t {
				if s, ok := e.(string); !ok || !short(s, 100) {
					return "", errors.New("choices are short texts")
				}
			}
		default:
			return "", errors.New("a value is a text, a number, yes / no or a list")
		}
		cur[k] = v
	}
	if len(cur) > 60 {
		return "", errors.New("too many values")
	}
	if len(cur) == 0 {
		return "", nil
	}
	b, _ := json.Marshal(cur)
	return string(b), nil
}

// ValueText writes a row's value of p as text (for exports and search);
// names maps user ids to names.
func ValueText(p DBProp, raw json.RawMessage, names map[string]string) string {
	if len(raw) == 0 {
		return ""
	}
	var x any
	if json.Unmarshal(raw, &x) != nil {
		return ""
	}
	option := func(id string) string {
		for _, o := range p.Options {
			if o.ID == id {
				return o.Name
			}
		}
		return id
	}
	switch v := x.(type) {
	case string:
		switch p.Type {
		case PropSelect:
			return option(v)
		case PropPerson:
			if n := names[v]; n != "" {
				return n
			}
		}
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "Yes"
		}
		return "No"
	case []any:
		var out []string
		for _, e := range v {
			s, _ := e.(string)
			switch p.Type {
			case PropMulti, PropSelect:
				s = option(s)
			case PropPerson:
				if n := names[s]; n != "" {
					s = n
				}
			}
			out = append(out, s)
		}
		return strings.Join(out, ", ")
	}
	return ""
}

// PropsOf reads a row's values.
func PropsOf(raw string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// DatabaseTable writes a database's rows as a Markdown table: the name,
// then each property (exports).
func DatabaseTable(db Database, rows []Row, names map[string]string) string {
	cell := func(s string) string {
		s = strings.ReplaceAll(strings.ReplaceAll(s, "|", `\|`), "\n", " ")
		if strings.TrimSpace(s) == "" {
			return " "
		}
		return s
	}
	head := []string{"Name"}
	for _, p := range db.Props {
		head = append(head, cell(p.Name))
	}
	lines := []string{"| " + strings.Join(head, " | ") + " |", "|" + strings.Repeat(" --- |", len(head))}
	rows = append([]Row(nil), rows...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Rank < rows[j].Rank })
	for _, r := range rows {
		vals := PropsOf(r.Props)
		line := []string{cell(r.Title)}
		for _, p := range db.Props {
			line = append(line, cell(ValueText(p, vals[p.ID], names)))
		}
		lines = append(lines, "| "+strings.Join(line, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

// Row is a database row for DatabaseTable.
type Row struct {
	Title string
	Props string
	Rank  float64
}

// PropLines writes a row's values as "Name: value" lines (exports, as
// Notion does).
func PropLines(db Database, props string, names map[string]string) string {
	vals := PropsOf(props)
	var out []string
	for _, p := range db.Props {
		if s := ValueText(p, vals[p.ID], names); s != "" {
			out = append(out, Escape(p.Name)+": "+s)
		}
	}
	return strings.Join(out, "  \n")
}
