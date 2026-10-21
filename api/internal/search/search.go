// Package search is Orbit's full-text index: pages, tasks, notes, events and
// task comments in one SQLite FTS5 table, ranked (bm25), accent-insensitive
// ("acao" finds "ação") and by word prefix ("rep" finds "report"). SQLite
// triggers keep it in step with every write — the app's, the git and
// calendar syncs', an import's — so nothing has to remember to index.
//
// The index is SQLite's. Where it can't be made (another engine, an SQLite
// without FTS5) Ready reports false and callers fall back to a plain search.
package search

import (
	"fmt"
	"strings"
	"sync/atomic"
	"unicode"

	"gorm.io/gorm"
)

// version: bump it to rebuild the index (when what's indexed changes).
const version = "1"

// Each source: its table, kind, and how one of its rows (written @) reads
// into the index columns: ref (what a hit opens — a comment opens its task),
// workspace, owner (personal notes), title and body.
var sources = []struct {
	table, kind, ref, ws, owner, title, body string
}{
	{"pages", "page", "@.id", "@.workspace_id", "''", "@.title", "@.content_text"},
	{"tasks", "task", "@.id", "@.workspace_id", "''", "@.title", "@.notes"},
	{"notes", "note", "@.id", "@.workspace_id", "@.owner_id", "@.title", "@.plain_text"},
	{"events", "event", "@.id", "@.workspace_id", "''", "@.title", "coalesce(@.location,'') || ' ' || coalesce(@.notes,'')"},
	{"task_comments", "comment", "@.task_id", "@.workspace_id", "''", "''", "@.body"},
}

var ready atomic.Bool

// Ready reports whether the index is there (Setup ran and worked).
func Ready() bool { return ready.Load() }

// Setup makes the index and its triggers (once), and fills it the first time
// (or after a version change). It reports whether search can use it.
func Setup(db *gorm.DB) bool {
	ok := db != nil && setup(db)
	ready.Store(ok)
	return ok
}

func setup(db *gorm.DB) bool {
	if db.Dialector.Name() != "sqlite" {
		return false
	}
	stmts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS search_index USING fts5(kind UNINDEXED, id UNINDEXED, ref UNINDEXED, workspace_id UNINDEXED, owner_id UNINDEXED, title, body, tokenize='unicode61 remove_diacritics 2')`,
		// Where each row sits in the index, to replace it without a scan.
		`CREATE TABLE IF NOT EXISTS search_rows (key TEXT PRIMARY KEY, rid INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS search_meta (version TEXT NOT NULL)`,
	}
	for _, s := range sources {
		at := func(expr, p string) string { return strings.ReplaceAll(expr, "@", p) }
		key := func(p string) string { return fmt.Sprintf("'%s:' || %s.id", s.kind, p) }
		del := func(p string) string {
			return fmt.Sprintf(`DELETE FROM search_index WHERE rowid = (SELECT rid FROM search_rows WHERE key = %s); DELETE FROM search_rows WHERE key = %s;`, key(p), key(p))
		}
		ins := func(p string) string {
			return fmt.Sprintf(`INSERT INTO search_index(kind, id, ref, workspace_id, owner_id, title, body) VALUES ('%s', %s.id, %s, coalesce(%s,''), coalesce(%s,''), coalesce(%s,''), coalesce(%s,'')); INSERT OR REPLACE INTO search_rows(key, rid) VALUES (%s, last_insert_rowid());`,
				s.kind, p, at(s.ref, p), at(s.ws, p), at(s.owner, p), at(s.title, p), at(s.body, p), key(p))
		}
		stmts = append(stmts,
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS search_%s_ai AFTER INSERT ON %s BEGIN %s END`, s.table, s.table, ins("new")),
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS search_%s_au AFTER UPDATE ON %s BEGIN %s %s END`, s.table, s.table, del("old"), ins("new")),
			fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS search_%s_ad AFTER DELETE ON %s BEGIN %s END`, s.table, s.table, del("old")),
		)
	}
	for _, q := range stmts {
		if err := db.Exec(q).Error; err != nil {
			return false
		}
	}
	var have string
	db.Raw(`SELECT version FROM search_meta LIMIT 1`).Scan(&have)
	if have != version {
		if Rebuild(db) != nil {
			return false
		}
	}
	return true
}

// Rebuild fills the index again from the tables.
func Rebuild(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, q := range []string{`DELETE FROM search_index`, `DELETE FROM search_rows`} {
			if err := tx.Exec(q).Error; err != nil {
				return err
			}
		}
		for _, s := range sources {
			// The trigger's statement, once per row, through an UPDATE that
			// changes nothing: reuses the same mapping.
			if err := tx.Exec(fmt.Sprintf(`UPDATE %s SET id = id`, s.table)).Error; err != nil {
				return err
			}
		}
		if err := tx.Exec(`DELETE FROM search_meta`).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO search_meta(version) VALUES (?)`, version).Error
	})
}

// Hit is one match: what it is and what it opens.
type Hit struct {
	Kind        string
	ID          string
	Ref         string
	WorkspaceID string
}

// Match turns what someone typed into an FTS5 query: every word, as a prefix
// ("rep" → rep*), all of them required. Quotes and operators are taken as
// text. "" when nothing searchable is left.
func Match(q string) string {
	var words []string
	for _, w := range strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if w != "" {
			words = append(words, `"`+w+`"*`)
		}
		if len(words) == 8 {
			break
		}
	}
	return strings.Join(words, " ")
}

// Find searches the workspaces given (and the user's own notes), best first.
func Find(db *gorm.DB, q string, wsIDs []string, uid string, limit int) ([]Hit, error) {
	m := Match(q)
	var out []Hit
	if m == "" {
		return out, nil
	}
	if len(wsIDs) == 0 {
		wsIDs = []string{""}
	}
	err := db.Raw(`SELECT kind, id, ref, workspace_id FROM search_index
		WHERE search_index MATCH ? AND (workspace_id IN ? OR (workspace_id = '' AND owner_id = ?))
		ORDER BY bm25(search_index, 0, 0, 0, 0, 0, 4.0, 1.0) LIMIT ?`, m, wsIDs, uid, limit).Scan(&out).Error
	return out, err
}
