package search

import (
	"fmt"
	"strings"
	"unicode"

	"gorm.io/gorm"
)

// PostgreSQL: search_index is a table with a tsvector of each row (its
// title weighing more than its body), filled by a trigger per source table
// — the same rows, the same mapping as SQLite's FTS5 index. search_fold
// takes accents off when the unaccent extension is there (else it's the
// text as it is).

func setupPostgres(db *gorm.DB) bool {
	fold := `CREATE OR REPLACE FUNCTION search_fold(t text) RETURNS text LANGUAGE sql IMMUTABLE AS $$ SELECT lower(coalesce(t, '')) $$`
	if db.Exec(`CREATE EXTENSION IF NOT EXISTS unaccent`).Error == nil {
		fold = `CREATE OR REPLACE FUNCTION search_fold(t text) RETURNS text LANGUAGE sql STABLE AS $$ SELECT lower(unaccent(coalesce(t, ''))) $$`
	}
	stmts := []string{
		fold,
		`CREATE TABLE IF NOT EXISTS search_index (kind text NOT NULL, id text NOT NULL, ref text NOT NULL, workspace_id text NOT NULL, owner_id text NOT NULL, title text NOT NULL, body text NOT NULL, doc tsvector NOT NULL, PRIMARY KEY (kind, id))`,
		`CREATE INDEX IF NOT EXISTS search_index_doc ON search_index USING gin (doc)`,
		`CREATE INDEX IF NOT EXISTS search_index_ws ON search_index (workspace_id)`,
		`CREATE TABLE IF NOT EXISTS search_meta (version TEXT NOT NULL)`,
	}
	for _, s := range sources {
		at := func(expr string) string { return strings.ReplaceAll(expr, "@", "NEW") }
		stmts = append(stmts,
			fmt.Sprintf(`CREATE OR REPLACE FUNCTION search_%[1]s_fn() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP <> 'INSERT' THEN DELETE FROM search_index WHERE kind = '%[2]s' AND id = OLD.id; END IF;
  IF TG_OP <> 'DELETE' THEN
    INSERT INTO search_index (kind, id, ref, workspace_id, owner_id, title, body, doc)
    VALUES ('%[2]s', NEW.id, %[3]s, coalesce(%[4]s, ''), coalesce(%[5]s, ''), coalesce(%[6]s, ''), coalesce(%[7]s, ''),
      setweight(to_tsvector('simple', search_fold(%[6]s)), 'A') || setweight(to_tsvector('simple', search_fold(%[7]s)), 'B'));
  END IF;
  RETURN NULL;
END $$`, s.table, s.kind, at(s.ref), at(s.ws), at(s.owner), at(s.title), at(s.body)),
			fmt.Sprintf(`DROP TRIGGER IF EXISTS search_%[1]s_t ON %[1]s`, s.table),
			fmt.Sprintf(`CREATE TRIGGER search_%[1]s_t AFTER INSERT OR UPDATE OR DELETE ON %[1]s FOR EACH ROW EXECUTE FUNCTION search_%[1]s_fn()`, s.table),
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

// tsQuery: every word typed, as a prefix, all of them required.
func tsQuery(q string) string {
	var words []string
	for _, w := range strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		words = append(words, w+":*")
		if len(words) == 8 {
			break
		}
	}
	return strings.Join(words, " & ")
}

func findPostgres(db *gorm.DB, q string, wsIDs []string, uid string, limit int) ([]Hit, error) {
	var out []Hit
	tq := tsQuery(q)
	if tq == "" {
		return out, nil
	}
	err := db.Raw(`SELECT kind, id, ref, workspace_id FROM search_index, to_tsquery('simple', search_fold(?)) AS query
		WHERE doc @@ query AND (workspace_id IN ? OR (workspace_id = '' AND owner_id = ?))
		ORDER BY ts_rank(doc, query) DESC LIMIT ?`, tq, wsIDs, uid, limit).Scan(&out).Error
	return out, err
}
