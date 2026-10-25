package api

import (
	"strconv"
	"time"
)

// stamp writes a time as the lists give it (UTC, milliseconds).
func stamp(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

// atoiSafe reads a number a query put in a text field (length(data)); 0 if none.
func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
