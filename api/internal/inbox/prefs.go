package inbox

import (
	"encoding/json"

	"github.com/joaoteixeira10/orbit/api/internal/models"
)

// Delivery is where one kind of notification goes besides the inbox.
type Delivery struct {
	Push     bool `json:"push"`     // this user's devices (Web Push)
	Channels bool `json:"channels"` // the channels they picked (email, ntfy, …)
}

// defaults: everything that is about you reaches your devices; others'
// activity stays in the inbox; channels only for what you turn on — except
// reminders, which keep the older "also send reminders" switch.
func defaults(p models.UserPrefs) map[string]Delivery {
	return map[string]Delivery{
		Reminder:  {Push: true, Channels: p.NotifyEvents},
		Assigned:  {Push: true},
		Comment:   {Push: true},
		Workspace: {Push: true},
		Activity:  {},
		Digest:    {Push: true},
	}
}

// PrefsOf is a user's choices per kind, defaults filled in.
func PrefsOf(p models.UserPrefs) map[string]Delivery {
	out := defaults(p)
	var stored map[string]Delivery
	if json.Unmarshal([]byte(p.NotifyPrefs), &stored) == nil {
		for _, k := range Kinds {
			if d, ok := stored[k]; ok {
				out[k] = d
			}
		}
	}
	return out
}

// EncodePrefs keeps only known kinds.
func EncodePrefs(in map[string]Delivery) string {
	clean := map[string]Delivery{}
	for _, k := range Kinds {
		if d, ok := in[k]; ok {
			clean[k] = d
		}
	}
	b, _ := json.Marshal(clean)
	return string(b)
}
