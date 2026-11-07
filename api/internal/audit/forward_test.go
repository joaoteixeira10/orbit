package audit

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.b.Write(p)
	l.b.WriteByte('\n')
	return len(p), nil
}
func (l *lockedBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

// TestForwardSendsCommittedEntries: each committed entry is forwarded once as
// JSON, with its detail and chain hash.
func TestForwardSendsCommittedEntries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.AuditLog{}); err != nil {
		t.Fatal(err)
	}
	r := New(db, zap.NewNop())
	var sink lockedBuf
	r.ForwardTo(&sink)
	r.Record("u1", "server.disable", "server", "gmr/facebook/web01:8001", "10.0.0.5", map[string]string{"reason": "deploy"})

	deadline := time.Now().Add(2 * time.Second)
	for !strings.Contains(sink.String(), "server.disable") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	line := strings.TrimSpace(strings.TrimPrefix(sink.String(), "orbit audit "))
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("forwarded line isn't JSON: %q (%v)", sink.String(), err)
	}
	if got["action"] != "server.disable" || got["ip"] != "10.0.0.5" || got["hash"] == "" {
		t.Errorf("forwarded = %v", got)
	}
	if d, _ := got["detail"].(map[string]any); d["reason"] != "deploy" {
		t.Errorf("detail = %v", got["detail"])
	}
}
