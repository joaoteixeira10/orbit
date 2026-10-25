package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

const (
	scheduleKey     = "scheduled_backup"
	scheduleLastKey = "scheduled_backup_last"   // RFC3339 of the last good run
	scheduleStatus  = "scheduled_backup_status" // JSON LastRun
	filePrefix      = "orbit-backup-masterkey-"
)

// Schedule is the owner's automatic backup: master-key archives (the same
// ORBIT_MASTER_KEY restores them) written to a directory (mount a volume) or an
// S3-compatible bucket, every IntervalHours, keeping the Keep newest.
type Schedule struct {
	Enabled       bool   `json:"enabled"`
	IntervalHours int    `json:"interval_hours"`
	Dest          string `json:"dest"` // "dir" (default) or "s3"
	Dir           string `json:"dir"`
	Keep          int    `json:"keep"` // 0 = keep all
	// S3: credentials come from the secrets backend (BACKUP_S3_ACCESS_KEY,
	// BACKUP_S3_SECRET_KEY), never from here.
	S3Bucket   string `json:"s3_bucket"`
	S3Prefix   string `json:"s3_prefix"`
	S3Endpoint string `json:"s3_endpoint"` // host[:port] (MinIO, …); empty = AWS
	S3Region   string `json:"s3_region"`
}

// LastRun is how the last scheduled backup went.
type LastRun struct {
	At    time.Time `json:"at"`
	OK    bool      `json:"ok"`
	Where string    `json:"where,omitempty"` // the file or s3:// location
	Error string    `json:"error,omitempty"`
}

// Validate checks a schedule before saving it.
func (s Schedule) Validate() error {
	if !s.Enabled {
		return nil
	}
	switch {
	case s.IntervalHours < 1 || s.IntervalHours > 24*31:
		return fmt.Errorf("interval: 1 to %d hours", 24*31)
	case s.Keep < 0 || s.Keep > 1000:
		return fmt.Errorf("keep: 0 (all) to 1000")
	case s.Dest != "" && s.Dest != "dir" && s.Dest != "s3":
		return fmt.Errorf("destination: dir or s3")
	case (s.Dest == "" || s.Dest == "dir") && !filepath.IsAbs(s.Dir):
		return fmt.Errorf("directory: an absolute path inside the container (mount a volume there), e.g. /backups")
	case s.Dest == "s3" && s.S3Bucket == "":
		return fmt.Errorf("S3: a bucket")
	}
	return nil
}

// LoadSchedule reads the schedule (off if unset).
func LoadSchedule(db *gorm.DB) Schedule {
	var row models.Setting
	var s Schedule
	if db.Where(&models.Setting{Key: scheduleKey}).First(&row).Error == nil {
		_ = json.Unmarshal([]byte(row.Value), &s)
	}
	return s
}

// SaveSchedule stores the schedule.
func SaveSchedule(db *gorm.DB, s Schedule) error {
	b, _ := json.Marshal(s)
	return db.Save(&models.Setting{Key: scheduleKey, Value: string(b), UpdatedAt: time.Now()}).Error
}

// LoadLastRun reads how the last scheduled backup went (zero if never).
func LoadLastRun(db *gorm.DB) LastRun {
	var row models.Setting
	var r LastRun
	if db.Where(&models.Setting{Key: scheduleStatus}).First(&row).Error == nil {
		_ = json.Unmarshal([]byte(row.Value), &r)
	}
	return r
}

func saveLastRun(db *gorm.DB, r LastRun) {
	b, _ := json.Marshal(r)
	db.Save(&models.Setting{Key: scheduleStatus, Value: string(b), UpdatedAt: time.Now()})
	if r.OK {
		db.Save(&models.Setting{Key: scheduleLastKey, Value: r.At.UTC().Format(time.RFC3339), UpdatedAt: time.Now()})
	}
}

func lastGood(db *gorm.DB) time.Time {
	var row models.Setting
	if db.Where(&models.Setting{Key: scheduleLastKey}).First(&row).Error != nil {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, row.Value)
	return t
}

// RunDueScheduled writes a backup when the schedule is on and its interval
// has passed since the last good one (call it often, e.g. hourly). It
// returns the run, or nil when nothing was due. A failure is retried at the
// next call.
func RunDueScheduled(db *gorm.DB, sealer *keystore.Sealer, appVersion string, now time.Time) *LastRun {
	s := LoadSchedule(db)
	if !s.Enabled || s.Validate() != nil || !Supported(db) {
		return nil
	}
	if last := lastGood(db); !last.IsZero() && now.Sub(last) < time.Duration(s.IntervalHours)*time.Hour {
		return nil
	}
	r := RunNow(db, sealer, s, appVersion, now)
	return &r
}

// RunNow writes one master-key backup to the schedule's destination.
func RunNow(db *gorm.DB, sealer *keystore.Sealer, s Schedule, appVersion string, now time.Time) LastRun {
	r := LastRun{At: now}
	archive, err := Export(db, sealer, "", appVersion, now)
	if err == nil {
		name := filePrefix + now.UTC().Format("20060102-150405") + ".json"
		if s.Dest == "s3" {
			r.Where, err = uploadS3(s, name, archive)
			if err == nil {
				pruneS3(s)
			}
		} else {
			r.Where, err = writeDir(s, name, archive)
		}
	}
	r.OK = err == nil
	if err != nil {
		r.Error = err.Error()
	}
	saveLastRun(db, r)
	return r
}

func writeDir(s Schedule, name string, archive []byte) (string, error) {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(s.Dir, name)
	if err := os.WriteFile(path, archive, 0o600); err != nil {
		return "", err
	}
	pruneDir(s.Dir, s.Keep)
	return path, nil
}

// pruneDir keeps the keep newest scheduled archives of dir.
func pruneDir(dir string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), filePrefix) && strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	if len(files) <= keep {
		return
	}
	sort.Strings(files) // timestamped names sort by time
	for _, n := range files[:len(files)-keep] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}
