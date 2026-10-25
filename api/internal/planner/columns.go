package planner

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
)

// Column is one status of a task list: a kanban column. Done columns hold
// completed tasks; a list always has at least one open and one done column.
type Column struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
	Done  bool   `json:"done"`
	// Limit is how many cards the column should hold at most (0: no limit);
	// the board shows when it's over, it doesn't refuse more.
	Limit int `json:"limit,omitempty"`
}

// Columns builds columns from names; the last one is the done column. IDs
// are assigned by CleanColumns when the list is saved.
func Columns(names ...string) []Column {
	out := make([]Column, len(names))
	for i, n := range names {
		out[i] = Column{Name: n, Done: i == len(names)-1}
	}
	return out
}

// DefaultColumns is what a new list gets when none are given.
func DefaultColumns() []Column { return Columns("To do", "Doing", "Done") }

func shortID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CleanColumns validates a list's columns: 1–12 of them, named, unique ids
// (missing ones are assigned), at least one open and one done column.
func CleanColumns(cols []Column) ([]Column, error) {
	if len(cols) == 0 || len(cols) > 12 {
		return nil, errors.New("a list needs between 1 and 12 columns")
	}
	seen := map[string]bool{}
	open, done := false, false
	out := make([]Column, 0, len(cols))
	for _, c := range cols {
		c.Name = strings.TrimSpace(c.Name)
		if c.Name == "" || len(c.Name) > 60 {
			return nil, errors.New("every column needs a name (up to 60 characters)")
		}
		if c.Color != "" && !validColor(c.Color) {
			return nil, errors.New("column colours are #rrggbb")
		}
		if c.Limit < 0 || c.Limit > 999 {
			return nil, errors.New("a column's limit is between 0 (none) and 999")
		}
		if c.ID == "" || seen[c.ID] || len(c.ID) > 36 {
			c.ID = shortID()
		}
		seen[c.ID] = true
		open = open || !c.Done
		done = done || c.Done
		out = append(out, c)
	}
	if !open || !done {
		return nil, errors.New("a list needs at least one open column and one done column")
	}
	return out, nil
}

// EncodeColumns / DecodeColumns convert the stored JSON.
func EncodeColumns(cols []Column) string {
	b, _ := json.Marshal(cols)
	return string(b)
}

func DecodeColumns(s string) []Column {
	var cols []Column
	if json.Unmarshal([]byte(s), &cols) != nil || len(cols) == 0 {
		return DefaultColumns()
	}
	return cols
}

// FirstOpen and FirstDone pick where a task goes when it is reopened or
// completed without a column being named.
func FirstOpen(cols []Column) Column {
	for _, c := range cols {
		if !c.Done {
			return c
		}
	}
	return cols[0]
}

func FirstDone(cols []Column) Column {
	for _, c := range cols {
		if c.Done {
			return c
		}
	}
	return cols[len(cols)-1]
}

// FindColumn returns the column with id.
func FindColumn(cols []Column, id string) (Column, bool) {
	for _, c := range cols {
		if c.ID == id {
			return c, true
		}
	}
	return Column{}, false
}

func validColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

// ValidColor reports a #rrggbb colour ("" is not valid here).
func ValidColor(s string) bool { return validColor(s) }
