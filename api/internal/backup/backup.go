// Package backup exports Orbit's database as one encrypted archive and
// restores it (from PKI Management's backup, adapted).
//
// Everything worth keeping is in the SQLite database — users (password
// hashes, sealed two-factor secrets, passkeys), roles, settings (sealed SSO
// and notification secrets), the audit chain, and the workspaces with their
// pages, notes, tasks, events and (sealed) calendar subscriptions — so the
// archive is a consistent snapshot of it (VACUUM INTO), gzipped and sealed
// with AES-256-GCM in a JSON envelope. Two modes:
//
//   - "master_key": sealed with a key derived from ORBIT_MASTER_KEY (the KEK
//     salt travels in the envelope). Restoring needs the same ORBIT_MASTER_KEY.
//   - "passphrase": sealed with a key derived from a passphrase (Argon2id,
//     its own salt), to keep or move the archive without the master key. The
//     secrets inside the database stay sealed with the instance's master key,
//     so a restore with another ORBIT_MASTER_KEY works but those secrets can't be
//     read (two-factor to set up again, SSO and notification secrets and
//     calendar subscriptions to re-enter).
//
// A restore writes the database file with Orbit stopped (orbit
// restore-backup); it never swaps the database of a running instance.
package backup

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"

	"gorm.io/gorm"
)

const (
	formatTag  = "orbit-backup"
	formatVer  = 1
	modePass   = "passphrase"
	modeMaster = "master_key"
	saltLen    = 16
	// MinPassphrase is the shortest passphrase accepted.
	MinPassphrase = 12
	// settingKEKSalt is where db.OpenSealer keeps the master key's salt.
	settingKEKSalt = "kek_salt"
)

// Envelope is the unencrypted outer JSON of an archive.
type Envelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	Mode       string `json:"mode"`
	AppVersion string `json:"app_version"`
	CreatedAt  string `json:"created_at"`
	KDFSalt    string `json:"kdf_salt"`   // base64: the passphrase's salt, or the master key's
	Ciphertext string `json:"ciphertext"` // base64(nonce || GCM(gzip(sqlite file)))
}

// Export snapshots the database and seals it: with the passphrase when one is
// given (portable), else with the instance's master-key sealer.
func Export(db *gorm.DB, sealer *keystore.Sealer, passphrase, appVersion string, now time.Time) ([]byte, error) {
	if passphrase != "" && len(passphrase) < MinPassphrase {
		return nil, fmt.Errorf("backup: the passphrase needs at least %d characters", MinPassphrase)
	}
	raw, err := snapshot(db)
	if err != nil {
		return nil, err
	}
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	env := Envelope{Format: formatTag, Version: formatVer, AppVersion: appVersion, CreatedAt: now.UTC().Format(time.RFC3339)}
	enc := sealer
	if passphrase != "" {
		salt, err := keystore.RandomSalt(saltLen)
		if err != nil {
			return nil, err
		}
		if enc, err = keystore.New(passphrase, salt); err != nil {
			return nil, err
		}
		env.Mode, env.KDFSalt = modePass, base64.StdEncoding.EncodeToString(salt)
	} else {
		var row models.Setting
		if err := db.Where(&models.Setting{Key: settingKEKSalt}).First(&row).Error; err != nil {
			return nil, errors.New("backup: the master key's salt is missing")
		}
		salt, err := base64.RawStdEncoding.DecodeString(row.Value)
		if err != nil {
			return nil, errors.New("backup: the master key's salt is corrupt")
		}
		env.Mode, env.KDFSalt = modeMaster, base64.StdEncoding.EncodeToString(salt)
	}
	ct, err := enc.Seal(gz.Bytes())
	if err != nil {
		return nil, err
	}
	env.Ciphertext = base64.StdEncoding.EncodeToString(ct)
	return json.MarshalIndent(env, "", "  ")
}

// ErrNotSQLite: the archives here hold an SQLite database; a PostgreSQL
// one is backed up with its own tools.
var ErrNotSQLite = errors.New("backups made here are of an SQLite database; on PostgreSQL use its own tools (pg_dump, or your provider's snapshots)")

// Supported reports whether this database can be backed up here.
func Supported(db *gorm.DB) bool { return db.Name() == "sqlite" }

// snapshot copies the live database consistently (VACUUM INTO a temp file).
func snapshot(db *gorm.DB) ([]byte, error) {
	if !Supported(db) {
		return nil, ErrNotSQLite
	}
	dir, err := os.MkdirTemp("", "orbit-backup-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "snapshot.db")
	if err := db.Exec("VACUUM INTO ?", path).Error; err != nil {
		return nil, fmt.Errorf("backup: snapshot: %w", err)
	}
	return os.ReadFile(filepath.Clean(path))
}

// Open reads an archive's envelope.
func Open(archive []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(archive, &env); err != nil {
		return env, errors.New("backup: not an Orbit backup (bad JSON)")
	}
	if env.Format != formatTag || env.Version != formatVer {
		return env, fmt.Errorf("backup: not an Orbit backup this version reads (%q v%d)", env.Format, env.Version)
	}
	return env, nil
}

// NeedsPassphrase reports whether an archive was sealed with a passphrase.
func (e Envelope) NeedsPassphrase() bool { return e.Mode == modePass }

// Decrypt returns the SQLite database inside an archive: with the passphrase
// (passphrase mode) or ORBIT_MASTER_KEY (master-key mode).
func Decrypt(archive []byte, passphrase, masterKey string) ([]byte, Envelope, error) {
	env, err := Open(archive)
	if err != nil {
		return nil, env, err
	}
	salt, err := base64.StdEncoding.DecodeString(env.KDFSalt)
	if err != nil || len(salt) == 0 {
		return nil, env, errors.New("backup: corrupt archive (salt)")
	}
	ct, err := base64.StdEncoding.DecodeString(env.Ciphertext)
	if err != nil {
		return nil, env, errors.New("backup: corrupt archive (ciphertext)")
	}
	var key string
	switch env.Mode {
	case modePass:
		if passphrase == "" {
			return nil, env, errors.New("backup: this archive needs its passphrase")
		}
		key = passphrase
	case modeMaster:
		if masterKey == "" {
			return nil, env, errors.New("backup: this archive needs ORBIT_MASTER_KEY")
		}
		key = masterKey
	default:
		return nil, env, fmt.Errorf("backup: unknown archive mode %q", env.Mode)
	}
	s, err := keystore.New(key, salt)
	if err != nil {
		return nil, env, err
	}
	gz, err := s.Unseal(ct)
	if err != nil {
		if env.Mode == modePass {
			return nil, env, errors.New("backup: wrong passphrase, or a corrupt archive")
		}
		return nil, env, errors.New("backup: ORBIT_MASTER_KEY doesn't match this archive, or it is corrupt")
	}
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, env, errors.New("backup: corrupt archive (gzip)")
	}
	raw, err := io.ReadAll(io.LimitReader(zr, 4<<30))
	if err != nil {
		return nil, env, errors.New("backup: corrupt archive (gzip)")
	}
	if !bytes.HasPrefix(raw, []byte("SQLite format 3\x00")) {
		return nil, env, errors.New("backup: the archive doesn't hold a database")
	}
	return raw, env, nil
}

// Restore writes the database inside an archive to dbPath. An existing file
// is refused unless force, and is then kept as <dbPath>.before-restore-<time>.
// Run it with Orbit stopped.
func Restore(archive []byte, passphrase, masterKey, dbPath string, force bool, now time.Time) (Envelope, string, error) {
	raw, env, err := Decrypt(archive, passphrase, masterKey)
	if err != nil {
		return env, "", err
	}
	kept := ""
	if _, err := os.Stat(dbPath); err == nil {
		if !force {
			return env, "", fmt.Errorf("backup: %s exists; stop Orbit and add --force to replace it (it is kept as a copy)", dbPath)
		}
		kept = dbPath + ".before-restore-" + now.UTC().Format("20060102-150405")
		if err := os.Rename(dbPath, kept); err != nil {
			return env, "", err
		}
		for _, side := range []string{"-wal", "-shm"} {
			_ = os.Rename(dbPath+side, kept+side)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return env, kept, err
	}
	tmp := dbPath + ".restoring"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return env, kept, err
	}
	return env, kept, os.Rename(tmp, dbPath)
}
