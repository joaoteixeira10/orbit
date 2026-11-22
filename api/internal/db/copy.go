package db

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/schema"

	"github.com/joaoteixeira10/orbit/api/internal/config"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/search"
)

// Copying a whole Orbit database into another — SQLite into PostgreSQL, or
// back (`orbit copy-db`). The target gets Orbit's tables (Migrate), then
// every row of every table, as it is: ids, times, files kept in the
// database, the sealed secrets (they open with the same ORBIT_MASTER_KEY).
// Then the search index is built again there. It's a copy, not a sync: Orbit
// is stopped meanwhile, and runs on the target afterwards.

// ErrNotEmpty: the target already holds an Orbit (copy with force to replace it).
var ErrNotEmpty = errors.New("the target database isn't empty")

// batchOf is how many rows go at once: fewer for tables of big rows.
func batchOf(m any) int {
	switch m.(type) {
	case *models.Blob, *models.CollabUpdate:
		return 20
	}
	return 500
}

// Copy copies src into dst, telling out what it does. With force, what dst
// holds is deleted first; without, a dst with any users is refused.
func Copy(src, dst *gorm.DB, force bool, out io.Writer) error {
	if err := Migrate(dst); err != nil {
		return fmt.Errorf("making the tables: %w", err)
	}
	all := models.AllModels()
	var users int64
	dst.Model(&models.User{}).Count(&users)
	if users > 0 && !force {
		return ErrNotEmpty
	}
	// Emptied first: what was there (forced), or what making the tables
	// wrote (the markers of its one-time migrations — the copy brings them).
	{
		for i := len(all) - 1; i >= 0; i-- {
			if err := dst.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(all[i]).Error; err != nil {
				return fmt.Errorf("emptying %s: %w", tableOf(dst, all[i]), err)
			}
		}
	}
	for _, m := range all {
		n, err := copyTable(src, dst, m)
		if err != nil {
			return fmt.Errorf("copying %s: %w", tableOf(src, m), err)
		}
		_, _ = fmt.Fprintf(out, "  %-28s %d\n", tableOf(src, m), n)
	}
	if dst.Name() == "postgres" {
		// Numbered rows go on after the copied numbers.
		if err := dst.Exec(`SELECT setval(pg_get_serial_sequence('collab_updates', 'id'), COALESCE(MAX(id), 1), MAX(id) IS NOT NULL) FROM collab_updates`).Error; err != nil {
			return fmt.Errorf("numbering: %w", err)
		}
	}
	if search.Setup(dst) {
		if err := search.Rebuild(dst); err != nil {
			return fmt.Errorf("the search index: %w", err)
		}
	}
	for _, t := range unknownTables(src, all) {
		_, _ = fmt.Fprintf(out, "  (not Orbit's, not copied: %s)\n", t)
	}
	return nil
}

// copyTable streams a table's rows into dst, in batches, every column as
// it is (zero values and all, no hooks, no associations).
func copyTable(src, dst *gorm.DB, m any) (int, error) {
	typ := reflect.TypeOf(m).Elem()
	rows, err := src.Model(m).Unscoped().Rows()
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	size := batchOf(m)
	buf := reflect.MakeSlice(reflect.SliceOf(typ), 0, size)
	put := dst.Session(&gorm.Session{SkipHooks: true, NewDB: true}).Select("*").Omit(clause.Associations)
	// gorm writes a column's default for a zero value (false, 0, "") even
	// when told to write it: those are set back after each batch.
	stmt := &gorm.Statement{DB: dst}
	if err := stmt.Parse(m); err != nil {
		return 0, err
	}
	var defaulted []*schema.Field
	for _, f := range stmt.Schema.Fields {
		if f.DBName != "" && f.DefaultValueInterface != nil && !reflect.ValueOf(f.DefaultValueInterface).IsZero() {
			defaulted = append(defaulted, f)
		}
	}
	fix := dst.Session(&gorm.Session{SkipHooks: true, NewDB: true})
	flush := func() error {
		if buf.Len() == 0 {
			return nil
		}
		// Which rows have such a zero (before gorm writes its default in).
		type unset struct {
			row int
			f   *schema.Field
		}
		var zeros []unset
		for i := 0; i < buf.Len(); i++ {
			for _, f := range defaulted {
				if _, zero := f.ValueOf(nil, buf.Index(i)); zero {
					zeros = append(zeros, unset{i, f})
				}
			}
		}
		p := reflect.New(buf.Type())
		p.Elem().Set(buf)
		if err := put.Create(p.Interface()).Error; err != nil {
			return err
		}
		for _, z := range zeros {
			row := p.Elem().Index(z.row).Addr().Interface()
			if err := fix.Model(row).UpdateColumn(z.f.DBName, reflect.Zero(z.f.FieldType).Interface()).Error; err != nil {
				return err
			}
		}
		buf = reflect.MakeSlice(reflect.SliceOf(typ), 0, size)
		return nil
	}
	n := 0
	for rows.Next() {
		v := reflect.New(typ)
		if err := src.ScanRows(rows, v.Interface()); err != nil {
			return n, err
		}
		buf = reflect.Append(buf, v.Elem())
		n++
		if buf.Len() == size {
			if err := flush(); err != nil {
				return n, err
			}
		}
	}
	if err := rows.Err(); err != nil {
		return n, err
	}
	return n, flush()
}

func tableOf(db *gorm.DB, m any) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return fmt.Sprintf("%T", m)
	}
	return stmt.Schema.Table
}

// unknownTables are src's tables Orbit doesn't know (left out of the copy).
func unknownTables(src *gorm.DB, all []any) []string {
	known := map[string]bool{}
	for _, m := range all {
		known[tableOf(src, m)] = true
	}
	tables, err := src.Migrator().GetTables()
	if err != nil {
		return nil
	}
	var out []string
	for _, t := range tables {
		if known[t] || strings.HasPrefix(t, "search_") || strings.HasPrefix(t, "sqlite_") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// OpenAt opens another database than the configured one (the target of a
// copy): a driver (sqlite or postgres) and its address.
func OpenAt(driver, url string) (*gorm.DB, error) {
	d, err := dialector(&config.Config{DBDriver: driver, DBURL: url})
	if err != nil {
		return nil, err
	}
	gdb, err := gorm.Open(d, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", driver, err)
	}
	return gdb, nil
}
