// Package feature is the instance-level feature-flag store ("beta" toggles),
// backed by a single Setting row. Flags default off; only the instance owner
// may change them. Both the API and the SPA gate behaviour on these. Flags are
// off by default except where Defaults says otherwise.
package feature

import (
	"encoding/json"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Flag names. Add new flags here with a default in Defaults.
const (
	// Sessions enables active-session/device management (list + revoke).
	Sessions = "sessions"
	// Impersonation enables admins holding users.impersonate to sign in as
	// another user to reproduce/diagnose what that user sees.
	Impersonation = "impersonation"
	// StepUp requires fresh re-authentication (password or passkey) before the
	// most dangerous actions (root-key/backup export, owner transfer, etc.).
	StepUp = "step_up_auth"
	// CalendarSubscriptions lets editors subscribe a workspace to outside ICS
	// calendars (Orbit then fetches those addresses on a timer). On by default.
	CalendarSubscriptions = "calendar_subscriptions"
	// CalendarFeeds lets users publish a private ICS address of their own
	// events and tasks for other calendar apps. On by default.
	CalendarFeeds = "calendar_feeds"
	// GitIntegration lets users connect their Gitea / Forgejo accounts and
	// sync repositories' issues with task lists. On by default.
	GitIntegration = "git_integration"
	// WorkspaceInvites lets workspace owners make invite links: whoever opens
	// one joins the workspace, creating an account if they have none. On by
	// default.
	WorkspaceInvites = "workspace_invites"
	// PersonalChannels lets each person add their own notification channels
	// (a Discord webhook, an ntfy topic…), which get the full text.
	PersonalChannels = "personal_channels"
	// CalDAVSync lets workspace owners keep the workspace's calendar in step
	// with a calendar on a CalDAV server (iCloud, Nextcloud…), both ways.
	CalDAVSync = "caldav_sync"
	// GuestLinks lets editors open a page (and what's under it) to anyone
	// with its link, without an account. On by default.
	GuestLinks = "guest_links"
	// PublicForms lets editors give a database's form a public link: anyone
	// with it can add a row. On by default.
	PublicForms = "public_forms"
)

const settingKey = "features"

// Defaults is the canonical set of flags and their default values (off,
// except the calendar and git integrations, which shipped on). The
// stored map is merged over this, so unknown stored keys are ignored and new
// flags appear automatically (off) without a migration.
var Defaults = map[string]bool{
	Sessions:              false,
	Impersonation:         false,
	StepUp:                false,
	CalendarSubscriptions: true,
	CalendarFeeds:         true,
	GitIntegration:        true,
	WorkspaceInvites:      true,
	PersonalChannels:      true,
	CalDAVSync:            true,
	GuestLinks:            true,
	PublicForms:           true,
}

// All returns every known flag with its effective value (stored value, else
// default). Always returns a fresh map safe to mutate.
func All(db *gorm.DB) map[string]bool {
	out := make(map[string]bool, len(Defaults))
	for k, v := range Defaults {
		out[k] = v
	}
	var s models.Setting
	if err := db.Where(&models.Setting{Key: settingKey}).First(&s).Error; err == nil && s.Value != "" {
		stored := map[string]bool{}
		if json.Unmarshal([]byte(s.Value), &stored) == nil {
			for k := range Defaults { // only honour known flags
				if v, ok := stored[k]; ok {
					out[k] = v
				}
			}
		}
	}
	return out
}

// Enabled reports whether a single flag is on.
func Enabled(db *gorm.DB, name string) bool {
	return All(db)[name]
}

// IsKnown reports whether name is a defined flag.
func IsKnown(name string) bool {
	_, ok := Defaults[name]
	return ok
}

// Set persists one flag. Unknown names are rejected by the caller (IsKnown).
func Set(db *gorm.DB, name string, on bool) error {
	cur := All(db)
	cur[name] = on
	b, err := json.Marshal(cur)
	if err != nil {
		return err
	}
	return db.Save(&models.Setting{Key: settingKey, Value: string(b)}).Error
}
