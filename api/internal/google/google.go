// Package google signs a workspace owner in with Google (OAuth 2.0 with
// PKCE, offline: a refresh token) so their Google Calendar can be kept in
// step with the workspace's — through Google's CalDAV interface, by the same
// syncer as any CalDAV calendar (caldav), with an access token instead of a
// password. The instance's owner registers the OAuth client (its id and
// secret, the secret sealed with the master key).
package google

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/joaoteixeira10/orbit/api/internal/keystore"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/safedial"

	"gorm.io/gorm"
)

// Google's addresses (tests point them at a fake).
var (
	AuthURL         = "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL        = "https://oauth2.googleapis.com/token" //nolint:gosec // an address, not a credential
	UserInfoURL     = "https://openidconnect.googleapis.com/v1/userinfo"
	CalendarListURL = "https://www.googleapis.com/calendar/v3/users/me/calendarList"
	CalDAVBase      = "https://apidata.googleusercontent.com/caldav/v2/"
	// CalendarAPI is the Calendar API (watching a calendar for changes).
	CalendarAPI = "https://www.googleapis.com/calendar/v3"
	// HTTP is the client for Google (public addresses only).
	HTTP = func() *http.Client { return safedial.PublicClient(30 * time.Second) }
)

// Scope: who they are (their address, shown), and their calendars.
const Scope = "openid email https://www.googleapis.com/auth/calendar"

const (
	keyClientID     = "google_client_id"
	keyClientSecret = "google_client_secret" // sealed, base64
)

// App is the instance's OAuth client at Google.
type App struct {
	ClientID     string
	ClientSecret string
}

// LoadApp reads the OAuth client (ok false when it isn't set up).
func LoadApp(db *gorm.DB, sealer *keystore.Sealer) (App, bool) {
	var rows []models.Setting
	for _, k := range []string{keyClientID, keyClientSecret} {
		var r models.Setting
		if db.Where(&models.Setting{Key: k}).First(&r).Error == nil {
			rows = append(rows, r)
		}
	}
	var app App
	for _, r := range rows {
		switch r.Key {
		case keyClientID:
			app.ClientID = r.Value
		case keyClientSecret:
			if raw, err := base64.StdEncoding.DecodeString(r.Value); err == nil {
				if plain, err := sealer.Unseal(raw); err == nil {
					app.ClientSecret = string(plain)
				}
			}
		}
	}
	return app, app.ClientID != "" && app.ClientSecret != ""
}

// SaveApp keeps the OAuth client; an empty secret keeps the one there is,
// an empty id removes it all.
func SaveApp(db *gorm.DB, sealer *keystore.Sealer, clientID, clientSecret string) error {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		for _, k := range []string{keyClientID, keyClientSecret} {
			if err := db.Where(&models.Setting{Key: k}).Delete(&models.Setting{}).Error; err != nil {
				return err
			}
		}
		return nil
	}
	if err := db.Save(&models.Setting{Key: keyClientID, Value: clientID, UpdatedAt: time.Now()}).Error; err != nil {
		return err
	}
	if clientSecret = strings.TrimSpace(clientSecret); clientSecret != "" {
		sealed, err := sealer.Seal([]byte(clientSecret))
		if err != nil {
			return err
		}
		return db.Save(&models.Setting{Key: keyClientSecret, Value: base64.StdEncoding.EncodeToString(sealed), UpdatedAt: time.Now()}).Error
	}
	return nil
}

// Challenge is PKCE's S256 of a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeURL is where to send the browser to sign in (offline: a refresh
// token comes back; consent asked again so it always does).
func AuthorizeURL(app App, redirect, state, verifier string) string {
	q := url.Values{
		"client_id":              {app.ClientID},
		"redirect_uri":           {redirect},
		"response_type":          {"code"},
		"scope":                  {Scope},
		"state":                  {state},
		"access_type":            {"offline"},
		"prompt":                 {"consent"},
		"include_granted_scopes": {"true"},
		"code_challenge":         {Challenge(verifier)},
		"code_challenge_method":  {"S256"},
	}
	return AuthURL + "?" + q.Encode()
}

// Token is what Google hands over.
type Token struct {
	Access  string
	Refresh string
	Expiry  time.Time
}

func tokenCall(ctx context.Context, form url.Values) (Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := HTTP().Do(req)
	if err != nil {
		return Token{}, unreachable(err)
	}
	defer func() { _ = res.Body.Close() }()
	var body struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Description  string `json:"error_description"`
	}
	_ = json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&body)
	if res.StatusCode != http.StatusOK || body.AccessToken == "" {
		msg := body.Description
		if msg == "" {
			msg = body.Error
		}
		if msg == "" {
			msg = res.Status
		}
		return Token{}, fmt.Errorf("signing in with Google was refused: %s", msg)
	}
	exp := time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	if body.ExpiresIn == 0 {
		exp = time.Now().Add(30 * time.Minute)
	}
	return Token{Access: body.AccessToken, Refresh: body.RefreshToken, Expiry: exp}, nil
}

// Exchange turns the sign-in's code into tokens.
func Exchange(ctx context.Context, app App, redirect, code, verifier string) (Token, error) {
	t, err := tokenCall(ctx, url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirect},
		"client_id": {app.ClientID}, "client_secret": {app.ClientSecret}, "code_verifier": {verifier},
	})
	if err == nil && t.Refresh == "" {
		return t, errors.New("no lasting access came back from Google (try again: it asks for consent)")
	}
	return t, err
}

// Refresh gets a fresh access token.
func Refresh(ctx context.Context, app App, refresh string) (Token, error) {
	return tokenCall(ctx, url.Values{
		"grant_type": {"refresh_token"}, "refresh_token": {refresh},
		"client_id": {app.ClientID}, "client_secret": {app.ClientSecret},
	})
}

func getJSON(ctx context.Context, access, addr string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	res, err := HTTP().Do(req)
	if err != nil {
		return unreachable(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		return refused(res.Status, raw)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(into)
}

// Email is the signed-in account's address.
func Email(ctx context.Context, access string) (string, error) {
	var u struct {
		Email string `json:"email"`
	}
	err := getJSON(ctx, access, UserInfoURL, &u)
	return u.Email, err
}

// Calendar is one of the account's calendars it may write to.
type Calendar struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Writable bool   `json:"writable"` // false: it may only be read (birthdays, one shared read-only)
}

// Calendars lists the account's calendars: the ones it can change first
// (its primary first of all), then the ones it may only read.
func Calendars(ctx context.Context, access string) ([]Calendar, error) {
	var page struct {
		Items []struct {
			ID              string `json:"id"`
			Summary         string `json:"summary"`
			SummaryOverride string `json:"summaryOverride"`
			BackgroundColor string `json:"backgroundColor"`
			AccessRole      string `json:"accessRole"`
			Primary         bool   `json:"primary"`
		} `json:"items"`
	}
	if err := getJSON(ctx, access, CalendarListURL+"?minAccessRole=reader&maxResults=250", &page); err != nil {
		return nil, err
	}
	var primary, writable, readOnly []Calendar
	for _, it := range page.Items {
		name := it.SummaryOverride
		if name == "" {
			name = it.Summary
		}
		c := Calendar{ID: it.ID, Name: name, Color: it.BackgroundColor, Writable: it.AccessRole == "owner" || it.AccessRole == "writer"}
		switch {
		case it.Primary:
			primary = append(primary, c)
		case c.Writable:
			writable = append(writable, c)
		default:
			readOnly = append(readOnly, c)
		}
	}
	return append(append(append([]Calendar{}, primary...), writable...), readOnly...), nil
}

// CalDAVURL is a calendar's collection on Google's CalDAV server.
func CalDAVURL(calendarID string) string {
	return CalDAVBase + url.PathEscape(calendarID) + "/events/"
}

// CalendarIDOf is the Google calendar a link's CalDAV address points at
// ("" when it isn't one).
func CalendarIDOf(caldavURL string) string {
	rest, ok := strings.CutPrefix(caldavURL, CalDAVBase)
	if !ok {
		return ""
	}
	rest, ok = strings.CutSuffix(rest, "/events/")
	if !ok {
		return ""
	}
	id, err := url.PathUnescape(rest)
	if err != nil {
		return ""
	}
	return id
}

// Channel is a watch on a calendar: Google calls Address on every change,
// with the channel's id and token, until Expiry.
type Channel struct {
	ID       string
	Resource string // Google's id of what is watched (to stop it)
	Expiry   time.Time
}

// WatchTTL is how long a watch is asked to last (Google may give less).
const WatchTTL = 7 * 24 * time.Hour

// Watch asks Google to call address on every change of a calendar's events.
func Watch(ctx context.Context, access, calendarID, channelID, address, token string) (Channel, error) {
	body := map[string]any{
		"id": channelID, "type": "web_hook", "address": address, "token": token,
		"params": map[string]string{"ttl": strconv.Itoa(int(WatchTTL / time.Second))},
	}
	var out struct {
		ID         string `json:"id"`
		ResourceID string `json:"resourceId"`
		Expiration string `json:"expiration"` // milliseconds since 1970, as text
	}
	if err := postJSON(ctx, access, CalendarAPI+"/calendars/"+url.PathEscape(calendarID)+"/events/watch", body, &out); err != nil {
		return Channel{}, err
	}
	ch := Channel{ID: out.ID, Resource: out.ResourceID, Expiry: time.Now().Add(WatchTTL)}
	if ms, err := strconv.ParseInt(out.Expiration, 10, 64); err == nil && ms > 0 {
		ch.Expiry = time.UnixMilli(ms)
	}
	return ch, nil
}

// StopWatch ends a watch (Google stops calling).
func StopWatch(ctx context.Context, access, channelID, resource string) error {
	return postJSON(ctx, access, CalendarAPI+"/channels/stop", map[string]string{"id": channelID, "resourceId": resource}, nil)
}

// postJSON sends a JSON body and reads the answer into into (nil: none). A
// refusal says Google's own reason.
func postJSON(ctx context.Context, access, addr string, body, into any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	res, err := HTTP().Do(req)
	if err != nil {
		return unreachable(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return refused(res.Status, raw)
	}
	if into == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, into)
}

// unreachable says why Google couldn't be reached (the network's reason:
// a name that doesn't resolve, a timeout, a refused or blocked address).
func unreachable(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err // without the method and address, said already
	}
	return fmt.Errorf("couldn't reach Google: %v", err)
}

// refused is Google saying no, in its own words when it gave them: an
// API's {"error":{"message"}} (e.g. "Google Calendar API has not been used
// in project … or it is disabled", "Request had insufficient authentication
// scopes"), or the sign-in's {"error","error_description"}.
func refused(status string, body []byte) error {
	var api struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &api) == nil && api.Error.Message != "" {
		return fmt.Errorf("refused by Google (%s): %s", status, api.Error.Message)
	}
	var oauth struct {
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if json.Unmarshal(body, &oauth) == nil && (oauth.Description != "" || oauth.Error != "") {
		msg := oauth.Description
		if msg == "" {
			msg = oauth.Error
		}
		return fmt.Errorf("refused by Google (%s): %s", status, msg)
	}
	return fmt.Errorf("refused by Google: %s", status)
}
