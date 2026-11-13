package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"log/syslog"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"go.uber.org/zap"
)

// forwardQueue bounds the entries waiting to be forwarded. Forwarding never
// blocks the action being audited: when the queue is full the entry is dropped
// from the forward (it is always in the database) and a warning is logged.
const forwardQueue = 256

// ForwardTo also sends every committed entry, as one JSON line, to sink (e.g.
// a SIEM via syslog). Call once at startup.
func (r *Recorder) ForwardTo(sink io.Writer) {
	r.forward = make(chan models.AuditLog, forwardQueue)
	go func() {
		for e := range r.forward {
			line, err := json.Marshal(forwardView(e))
			if err != nil {
				continue
			}
			if _, err := sink.Write(append([]byte("orbit audit "), line...)); err != nil {
				r.log.Warn("audit forward failed", zap.Error(err))
			}
		}
	}()
}

// ForwardToSyslog connects to addr — "host:port" (UDP) or a Unix socket path
// such as /dev/log — and forwards every entry there (facility AUTH, INFO).
func (r *Recorder) ForwardToSyslog(addr string) error {
	network := "udp"
	if strings.HasPrefix(addr, "/") {
		network = "unixgram"
	} else if !strings.Contains(addr, ":") {
		addr += ":514"
	}
	w, err := syslog.Dial(network, addr, syslog.LOG_INFO|syslog.LOG_AUTH, "orbit")
	if err != nil {
		return fmt.Errorf("syslog %s: %w", addr, err)
	}
	r.ForwardTo(w)
	return nil
}

func (r *Recorder) enqueueForward(e models.AuditLog) {
	if r.forward == nil {
		return
	}
	select {
	case r.forward <- e:
	default:
		r.log.Warn("audit forward queue full; entry kept in the database only", zap.String("action", e.Action))
	}
}

// forwardView is the forwarded shape: the entry plus its chain hash (so a SIEM
// copy can be matched against /audit/verify).
func forwardView(e models.AuditLog) map[string]any {
	return map[string]any{
		"id": e.ID, "time": e.CreatedAt.UTC(), "user_id": e.UserID, "impersonator_id": e.ImpersonatorID,
		"action": e.Action, "target_type": e.TargetType, "target_id": e.TargetID, "ip": e.IP,
		"detail": json.RawMessage(orNull(e.Detail)), "hash": e.Hash,
	}
}

func orNull(s string) string {
	if s == "" {
		return "null"
	}
	return s
}
