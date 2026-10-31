package backup

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/secrets"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// s3Keys are the backup bucket's credentials, from the secrets backend.
func s3Keys() (string, string, error) {
	prov, err := secrets.New(os.Getenv("SECRETS_BACKEND"))
	if err != nil {
		return "", "", err
	}
	ak, _ := prov.Get("BACKUP_S3_ACCESS_KEY")
	sk, _ := prov.Get("BACKUP_S3_SECRET_KEY")
	if ak == "" || sk == "" {
		return "", "", fmt.Errorf("S3 backups need BACKUP_S3_ACCESS_KEY and BACKUP_S3_SECRET_KEY")
	}
	return ak, sk, nil
}

// s3Client builds an S3 client; the credentials come from the secrets backend
// (BACKUP_S3_ACCESS_KEY, BACKUP_S3_SECRET_KEY). An empty endpoint is AWS.
func s3Client(s Schedule) (*minio.Client, error) {
	ak, sk, err := s3Keys()
	if err != nil {
		return nil, err
	}
	endpoint := s.S3Endpoint
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	return minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(ak, sk, ""), Secure: true, Region: s.S3Region})
}

func s3Key(s Schedule, name string) string {
	p := strings.Trim(s.S3Prefix, "/")
	if p != "" {
		p += "/"
	}
	return p + name
}

// uploadS3 puts the archive in the bucket and returns its s3:// location.
func uploadS3(s Schedule, name string, data []byte) (string, error) {
	cli, err := s3Client(s)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	key := s3Key(s, name)
	if _, err := cli.PutObject(ctx, s.S3Bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: "application/json"}); err != nil {
		return "", fmt.Errorf("S3 upload: %w", err)
	}
	return "s3://" + s.S3Bucket + "/" + key, nil
}

// pruneS3 keeps the Keep newest scheduled archives under the prefix.
func pruneS3(s Schedule) {
	if s.Keep <= 0 {
		return
	}
	cli, err := s3Client(s)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var keys []string
	for obj := range cli.ListObjects(ctx, s.S3Bucket, minio.ListObjectsOptions{Prefix: s3Key(s, filePrefix)}) {
		if obj.Err == nil {
			keys = append(keys, obj.Key)
		}
	}
	if len(keys) <= s.Keep {
		return
	}
	sort.Strings(keys)
	for _, k := range keys[:len(keys)-s.Keep] {
		_ = cli.RemoveObject(ctx, s.S3Bucket, k, minio.RemoveObjectOptions{})
	}
}
