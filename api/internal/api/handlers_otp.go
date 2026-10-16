package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/auth"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"github.com/gin-gonic/gin"
)

type otpVerifyRequest struct {
	Code string `json:"code" binding:"required"`
}

const backupCodeCount = 10

// genBackupCodes returns plaintext recovery codes and their sha256 hashes.
func genBackupCodes() (plain, hashes []string) {
	for i := 0; i < backupCodeCount; i++ {
		b := make([]byte, 5)
		_, _ = rand.Read(b)
		h := hex.EncodeToString(b) // 10 hex chars
		code := h[:5] + "-" + h[5:]
		plain = append(plain, code)
		hashes = append(hashes, hashBackupCode(code))
	}
	return
}

func normalizeBackupCode(code string) string {
	return strings.ToLower(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(code)))
}

func hashBackupCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeBackupCode(code)))
	return hex.EncodeToString(sum[:])
}

// consumeBackupCode checks code against the user's stored hashes; on a match it
// removes that hash (single-use) and persists, returning true.
func (h *Handlers) consumeBackupCode(u *models.User, code string) bool {
	var hashes []string
	if u.OTPBackupCodes != "" {
		_ = json.Unmarshal([]byte(u.OTPBackupCodes), &hashes)
	}
	want := hashBackupCode(code)
	for i, hh := range hashes {
		if hh == want {
			hashes = append(hashes[:i], hashes[i+1:]...)
			b, _ := json.Marshal(hashes)
			h.db.Model(u).Update("otp_backup_codes", string(b))
			return true
		}
	}
	return false
}

func backupRemaining(u models.User) int {
	var hashes []string
	if u.OTPBackupCodes != "" {
		_ = json.Unmarshal([]byte(u.OTPBackupCodes), &hashes)
	}
	return len(hashes)
}

// setupOTP generates a new TOTP secret for the user (pending until verified)
// and returns the secret + otpauth URL for a QR code.
//
//	@Summary	Begin TOTP enrollment
//	@Tags		auth
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	map[string]string
//	@Router		/me/otp/setup [post]
func (h *Handlers) setupOTP(c *gin.Context) {
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	// While two-factor is on, a new setup would silently turn it off: it's
	// turned off first, with a code (disableOTP).
	if user.OTPEnabled {
		c.JSON(http.StatusConflict, gin.H{"error": "two-factor is already on; turn it off first (with a code) to set up a new authenticator"})
		return
	}
	secret, url, err := auth.GenerateTOTP("Orbit", user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate secret"})
		return
	}
	sealed, err := h.sealer.Seal([]byte(secret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store secret"})
		return
	}
	// Store the pending secret but keep OTP disabled until a code is verified.
	if err := h.db.Model(&user).Updates(map[string]any{"otp_secret_enc": sealed, "otp_enabled": false}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store secret"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"secret": secret, "otpauth_url": url})
}

// verifyOTP confirms a code against the pending secret and enables TOTP.
//
//	@Summary	Verify and enable TOTP
//	@Tags		auth
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	otpVerifyRequest	true	"TOTP code"
//	@Success	204
//	@Failure	400	{object}	map[string]string
//	@Router		/me/otp/verify [post]
func (h *Handlers) verifyOTP(c *gin.Context) {
	var req otpVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	secret, ok := h.unsealOTP(user)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no pending enrollment; start setup first"})
		return
	}
	if !auth.ValidateTOTP(secret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}
	plain, hashes := genBackupCodes()
	hb, _ := json.Marshal(hashes)
	if err := h.db.Model(&user).Updates(map[string]any{"otp_enabled": true, "otp_backup_codes": string(hb)}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not enable OTP"})
		return
	}
	h.audit.Record(user.ID, "auth.otp_enable", "user", user.ID, c.ClientIP(), nil)
	// Recovery codes are shown exactly once.
	c.JSON(http.StatusOK, gin.H{"backup_codes": plain})
}

// regenerateBackupCodes issues a fresh set of recovery codes after verifying a
// current TOTP code, invalidating the old ones.
//
//	@Summary	Regenerate recovery codes
//	@Tags		auth
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		otpVerifyRequest	true	"Current TOTP code"
//	@Success	200		{object}	map[string][]string
//	@Router		/me/otp/backup-codes [post]
func (h *Handlers) regenerateBackupCodes(c *gin.Context) {
	var req otpVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !user.OTPEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "two-factor is not enabled"})
		return
	}
	secret, ok := h.unsealOTP(user)
	if !ok || !auth.ValidateTOTP(secret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}
	plain, hashes := genBackupCodes()
	hb, _ := json.Marshal(hashes)
	h.db.Model(&user).Update("otp_backup_codes", string(hb))
	h.audit.Record(user.ID, "auth.otp_backup_regenerate", "user", user.ID, c.ClientIP(), nil)
	c.JSON(http.StatusOK, gin.H{"backup_codes": plain})
}

// disableOTP turns off TOTP after verifying a current code. Not allowed while
// OTP is globally enforced.
//
//	@Summary	Disable TOTP
//	@Tags		auth
//	@Accept		json
//	@Security	BearerAuth
//	@Param		body	body	otpVerifyRequest	true	"Current TOTP code"
//	@Success	204
//	@Failure	400	{object}	map[string]string
//	@Router		/me/otp/disable [post]
func (h *Handlers) disableOTP(c *gin.Context) {
	if h.otpEnforced() || h.otpRequiredFor(auth.UserID(c)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "two-factor is enforced by an administrator"})
		return
	}
	var req otpVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	var user models.User
	if err := h.db.First(&user, "id = ?", auth.UserID(c)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if !user.OTPEnabled {
		c.Status(http.StatusNoContent)
		return
	}
	secret, ok := h.unsealOTP(user)
	if !ok || !auth.ValidateTOTP(secret, req.Code) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid code"})
		return
	}
	if err := h.db.Model(&user).Updates(map[string]any{"otp_enabled": false, "otp_secret_enc": nil, "otp_backup_codes": ""}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not disable OTP"})
		return
	}
	h.audit.Record(user.ID, "auth.otp_disable", "user", user.ID, c.ClientIP(), nil)
	c.Status(http.StatusNoContent)
}

// unsealOTP returns the user's decrypted TOTP secret.
func (h *Handlers) unsealOTP(u models.User) (string, bool) {
	if len(u.OTPSecretEnc) == 0 {
		return "", false
	}
	b, err := h.sealer.Unseal(u.OTPSecretEnc)
	if err != nil {
		return "", false
	}
	return string(b), true
}
