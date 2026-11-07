// Package google signs a workspace owner in with Google (OAuth 2.0 with
// PKCE, offline: a refresh token) so their Google Calendar can be kept in
// step with the workspace's — through Google's CalDAV interface, by the same
// syncer as any CalDAV calendar (caldav), with an access token instead of a
// password. The instance's owner registers the OAuth client (its id and
// secret, the secret sealed with the master key).
package google

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
		return Token{}, errors.New("couldn't reach Google")
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
		return errors.New("couldn't reach Google")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("the answer from Google was %s", res.Status)
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
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Calendars lists the calendars the account can change (owner or writer).
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
	if err := getJSON(ctx, access, CalendarListURL+"?minAccessRole=writer&maxResults=250", &page); err != nil {
		return nil, err
	}
	out := []Calendar{}
	for _, it := range page.Items {
		name := it.SummaryOverride
		if name == "" {
			name = it.Summary
		}
		c := Calendar{ID: it.ID, Name: name, Color: it.BackgroundColor}
		if it.Primary {
			out = append([]Calendar{c}, out...)
		} else {
			out = append(out, c)
		}
	}
	return out, nil
}

// CalDAVURL is a calendar's collection on Google's CalDAV server.
func CalDAVURL(calendarID string) string {
	return CalDAVBase + url.PathEscape(calendarID) + "/events/"
}
