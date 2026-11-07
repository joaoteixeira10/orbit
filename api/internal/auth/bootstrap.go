package auth

import (
	"crypto/rand"
	"fmt"
	"strings"

	"github.com/joaoteixeira10/orbit/api/internal/models"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

// tempAlphabet leaves out look-alikes (0/o, 1/l/i).
const tempAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// TempPassword returns a one-time password like "k7mq-8ra2-xw4n-p3de" (~80 bits).
func TempPassword() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	var sb strings.Builder
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			sb.WriteByte('-')
		}
		sb.WriteByte(tempAlphabet[int(c)%len(tempAlphabet)])
	}
	return sb.String()
}

// Bootstrap ensures an initial admin account exists.
//
// On first run (no users in the DB) it creates ADMIN_USER (default "admin") as
// the instance owner with the built-in admin role. The password is
// ADMIN_PASSWORD, or — when that is empty — a random temporary password that
// is printed to the log once. Either way it must be changed at first sign-in.
// If users already exist it does nothing. Run after rbac.SeedBuiltins.
func Bootstrap(db *gorm.DB, log *zap.Logger, username, password string) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if username == "" {
		username = "admin"
	}
	generated := password == ""
	if generated {
		password = TempPassword()
	}
	var adminRole models.Role
	if err := db.Where("name = ? AND builtin = ?", string(models.RoleAdmin), true).First(&adminRole).Error; err != nil {
		return fmt.Errorf("cannot bootstrap admin: built-in admin role missing: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	admin := models.User{
		Username:           username,
		Owner:              true, // the bootstrap admin is the instance owner
		PasswordHash:       hash,
		MustChangePassword: true,
		Roles:              []models.Role{adminRole},
		Prefs:              models.UserPrefs{Timezone: "local", Theme: models.ThemeSystem},
	}
	if err := db.Create(&admin).Error; err != nil {
		return err
	}
	if generated {
		// Printed once, on purpose: this is how the first admin gets in. Also
		// as plain text on stdout so `docker compose logs | grep` finds it.
		fmt.Printf("\n  First sign-in: user %s, temporary password %s\n  (you'll choose a new password)\n\n", username, password)
		log.Warn("bootstrapped initial admin with a temporary password (printed above; change it at first sign-in)",
			zap.String("username", username))
	} else {
		log.Warn("bootstrapped initial admin from ADMIN_PASSWORD — you'll be asked to change it at first sign-in",
			zap.String("username", username))
	}
	return nil
}

// EnsureOwner guarantees exactly one instance owner exists. No-op once an owner
// is set (fresh installs get one from Bootstrap); for pre-existing installs it
// backfills the earliest superuser (falling back to the earliest user) so the
// owner-only powers have a holder. Idempotent; run after roles are assigned.
func EnsureOwner(db *gorm.DB, log *zap.Logger) error {
	var owners int64
	if err := db.Model(&models.User{}).Where("owner = ?", true).Count(&owners).Error; err != nil {
		return err
	}
	if owners > 0 {
		return nil
	}
	var ids []string
	db.Table("user_roles").
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.superuser = ?", true).
		Order("user_roles.user_id").
		Limit(1).
		Pluck("user_roles.user_id", &ids)

	var u models.User
	if len(ids) > 0 {
		if err := db.First(&u, "id = ?", ids[0]).Error; err != nil {
			return err
		}
	} else if err := db.Order("id").First(&u).Error; err != nil {
		return nil // no users at all yet — nothing to do
	}
	if err := db.Model(&models.User{}).Where("id = ?", u.ID).Update("owner", true).Error; err != nil {
		return err
	}
	log.Info("assigned instance owner (backfill)", zap.String("user_id", u.ID), zap.String("username", u.Username))
	return nil
}
