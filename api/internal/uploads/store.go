package uploads

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Where files' bytes are kept: by their SHA-256, once each (the same file
// added twice is kept once), in the database (the default: a backup carries
// them), a directory, or an S3 bucket. The bytes go when the last file using
// them does.

// Store keeps bytes by their SHA-256 (hex). db is the database (or the
// transaction) in use; only the database store writes to it.
type Store interface {
	Kind() string // db | dir | s3
	Put(db *gorm.DB, sum string, data []byte) error
	Get(db *gorm.DB, sum string) ([]byte, error)
	Delete(db *gorm.DB, sum string) error
	// Each calls fn with every sum kept (stopping at its first error).
	Each(db *gorm.DB, fn func(sum string) error) error
}

// The store kinds.
const (
	StoreDB  = "db"
	StoreDir = "dir"
	StoreS3  = "s3"
)

// ErrMissing: no bytes kept for that sum.
var ErrMissing = errors.New("the file's bytes are missing")

var current Store = DBStore{}

// Use makes s the store files are kept in (at start).
func Use(s Store) { current = s }

// Current is the store files are kept in.
func Current() Store { return current }

var hexSum = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidSum reports a well-formed SHA-256 (hex), the only names stores use.
func ValidSum(sum string) bool { return hexSum.MatchString(sum) }

// ---- The database ----

// DBStore keeps the bytes in the database (table blobs).
type DBStore struct{}

// Kind is "db".
func (DBStore) Kind() string { return StoreDB }

// Put keeps data (nothing if those bytes are there already).
func (DBStore) Put(db *gorm.DB, sum string, data []byte) error {
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.Blob{SHA256: sum, Data: data}).Error
}

// Get reads the bytes.
func (DBStore) Get(db *gorm.DB, sum string) ([]byte, error) {
	var b models.Blob
	if err := db.First(&b, "sha256 = ?", sum).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMissing
		}
		return nil, err
	}
	return b.Data, nil
}

// Delete removes the bytes.
func (DBStore) Delete(db *gorm.DB, sum string) error {
	return db.Delete(&models.Blob{}, "sha256 = ?", sum).Error
}

// Each lists the sums kept.
func (DBStore) Each(db *gorm.DB, fn func(string) error) error {
	var sums []string
	if err := db.Model(&models.Blob{}).Order("sha256").Pluck("sha256", &sums).Error; err != nil {
		return err
	}
	for _, s := range sums {
		if err := fn(s); err != nil {
			return err
		}
	}
	return nil
}

// ---- A directory ----

// DirStore keeps each file as <Root>/<first two of the sum>/<sum>; files
// never change once written, so any copy tool backs the directory up.
type DirStore struct{ Root string }

// Kind is "dir".
func (DirStore) Kind() string { return StoreDir }

func (s DirStore) path(sum string) (string, error) {
	if !ValidSum(sum) {
		return "", fmt.Errorf("uploads: bad sum %q", sum)
	}
	return filepath.Join(s.Root, sum[:2], sum), nil
}

// Put writes data (nothing if the file is there already), through a
// temporary file so a half-written one is never read.
func (s DirStore) Put(_ *gorm.DB, sum string, data []byte) error {
	p, err := s.path(sum)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".put-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp.Name())
		return errors.Join(werr, cerr)
	}
	return os.Rename(tmp.Name(), p)
}

// Get reads the file.
func (s DirStore) Get(_ *gorm.DB, sum string) ([]byte, error) {
	p, err := s.path(sum)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p) //nolint:gosec // a checked sum under the configured root
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrMissing
	}
	return b, err
}

// Delete removes the file (gone already is fine).
func (s DirStore) Delete(_ *gorm.DB, sum string) error {
	p, err := s.path(sum)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Each lists the files kept.
func (s DirStore) Each(_ *gorm.DB, fn func(string) error) error {
	dirs, err := os.ReadDir(s.Root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() || len(d.Name()) != 2 {
			continue
		}
		files, err := os.ReadDir(filepath.Join(s.Root, d.Name()))
		if err != nil {
			return err
		}
		for _, f := range files {
			if ValidSum(f.Name()) {
				if err := fn(f.Name()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ---- An S3 bucket ----

// S3Config says where an S3-compatible bucket is. An empty Endpoint is AWS.
type S3Config struct {
	Bucket, Prefix, Endpoint, Region string
	AccessKey, SecretKey             string
	Insecure                         bool // plain HTTP (a MinIO in the lab)
}

// S3Store keeps each file as <Prefix>/<first two of the sum>/<sum>.
type S3Store struct {
	cli    *minio.Client
	bucket string
	prefix string
}

// NewS3 connects to the bucket (it isn't checked until used).
func NewS3(c S3Config) (*S3Store, error) {
	if c.Bucket == "" {
		return nil, errors.New("uploads: S3 needs a bucket (UPLOAD_S3_BUCKET)")
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return nil, errors.New("uploads: S3 needs UPLOAD_S3_ACCESS_KEY and UPLOAD_S3_SECRET_KEY")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	cli, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(c.AccessKey, c.SecretKey, ""), Secure: !c.Insecure, Region: c.Region})
	if err != nil {
		return nil, err
	}
	p := strings.Trim(c.Prefix, "/")
	if p != "" {
		p += "/"
	}
	return &S3Store{cli: cli, bucket: c.Bucket, prefix: p}, nil
}

// Kind is "s3".
func (*S3Store) Kind() string { return StoreS3 }

func (s *S3Store) key(sum string) (string, error) {
	if !ValidSum(sum) {
		return "", fmt.Errorf("uploads: bad sum %q", sum)
	}
	return s.prefix + sum[:2] + "/" + sum, nil
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Minute)
}

// Put uploads data (nothing if the object is there already).
func (s *S3Store) Put(_ *gorm.DB, sum string, data []byte) error {
	k, err := s.key(sum)
	if err != nil {
		return err
	}
	c, cancel := ctx()
	defer cancel()
	if _, err := s.cli.StatObject(c, s.bucket, k, minio.StatObjectOptions{}); err == nil {
		return nil
	}
	_, err = s.cli.PutObject(c, s.bucket, k, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: "application/octet-stream"})
	return err
}

// Get downloads the object.
func (s *S3Store) Get(_ *gorm.DB, sum string) ([]byte, error) {
	k, err := s.key(sum)
	if err != nil {
		return nil, err
	}
	c, cancel := ctx()
	defer cancel()
	obj, err := s.cli.GetObject(c, s.bucket, k, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = obj.Close() }()
	b, err := io.ReadAll(obj)
	if err != nil && minio.ToErrorResponse(err).Code == "NoSuchKey" {
		return nil, ErrMissing
	}
	return b, err
}

// Delete removes the object.
func (s *S3Store) Delete(_ *gorm.DB, sum string) error {
	k, err := s.key(sum)
	if err != nil {
		return err
	}
	c, cancel := ctx()
	defer cancel()
	return s.cli.RemoveObject(c, s.bucket, k, minio.RemoveObjectOptions{})
}

// Each lists the objects kept under the prefix.
func (s *S3Store) Each(_ *gorm.DB, fn func(string) error) error {
	c, cancel := context.WithCancel(context.Background())
	defer cancel()
	for obj := range s.cli.ListObjects(c, s.bucket, minio.ListObjectsOptions{Prefix: s.prefix, Recursive: true}) {
		if obj.Err != nil {
			return obj.Err
		}
		if sum := obj.Key[strings.LastIndex(obj.Key, "/")+1:]; ValidSum(sum) {
			if err := fn(sum); err != nil {
				return err
			}
		}
	}
	return nil
}
