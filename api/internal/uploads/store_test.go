package uploads

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"

	"github.com/minio/minio-go/v7"
)

// TestS3Store: put (twice), get, list, delete against a real bucket — run
// with ORBIT_TEST_S3=host:port of an S3 server over HTTP, keys minioadmin /
// minioadmin (a MinIO, or `rclone serve s3 /data --auth-key minioadmin,minioadmin`).
func TestS3Store(t *testing.T) {
	ep := os.Getenv("ORBIT_TEST_S3")
	if ep == "" {
		t.Skip("ORBIT_TEST_S3 not set")
	}
	s, err := NewS3(S3Config{Bucket: "orbit-test", Prefix: "/files/", Endpoint: ep, AccessKey: "minioadmin", SecretKey: "minioadmin", Insecure: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.cli.MakeBucket(context.Background(), "orbit-test", minio.MakeBucketOptions{}); err != nil {
		t.Logf("bucket: %v", err)
	}
	data := []byte("hello, bucket")
	sum := Sum(data)
	for i := 0; i < 2; i++ { // the second is a no-op
		if err := s.Put(nil, sum, data); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Get(nil, sum)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("get: %q %v", got, err)
	}
	var seen []string
	if err := s.Each(nil, func(x string) error { seen = append(seen, x); return nil }); err != nil || len(seen) != 1 || seen[0] != sum {
		t.Fatalf("each: %v %v", seen, err)
	}
	if err := s.Delete(nil, sum); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(nil, sum); !errors.Is(err, ErrMissing) {
		t.Fatalf("after delete: %v", err)
	}
	if err := s.Put(nil, "../etc/passwd", data); err == nil {
		t.Fatal("a bad sum was accepted")
	}
}

// TestDirStoreRejectsPaths: only a SHA-256 names a file.
func TestDirStoreRejectsPaths(t *testing.T) {
	s := DirStore{Root: t.TempDir()}
	for _, bad := range []string{"../x", "", "ab/../../c", "ABCDEF"} {
		if err := s.Put(nil, bad, []byte("x")); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := s.Get(nil, Sum([]byte("nothing"))); !errors.Is(err, ErrMissing) {
		t.Errorf("a missing file: %v", err)
	}
}
