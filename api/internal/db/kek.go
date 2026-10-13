package db

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// settingKEKSalt stores the random Argon2id salt for the key-encryption key.
const settingKEKSalt = "kek_salt"

// OpenSealer returns the keystore sealer for ORBIT_MASTER_KEY: the KEK is derived
// with Argon2id over a random salt kept in the settings table, created on the
// first start. (PKI Management also migrated a legacy SHA-256 KEK here; LB
// Admin never had one.) The master key must never change afterwards, or the
// sealed secrets can't be read.
func OpenSealer(gdb *gorm.DB, masterKey string) (*keystore.Sealer, error) {
	var row models.Setting
	if err := gdb.Where(&models.Setting{Key: settingKEKSalt}).First(&row).Error; err == nil && row.Value != "" {
		salt, derr := base64.RawStdEncoding.DecodeString(row.Value)
		if derr != nil {
			return nil, fmt.Errorf("kek: corrupt stored salt: %w", derr)
		}
		return keystore.New(masterKey, salt)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if err := gdb.Save(&models.Setting{Key: settingKEKSalt, Value: base64.RawStdEncoding.EncodeToString(salt), UpdatedAt: time.Now()}).Error; err != nil {
		return nil, fmt.Errorf("kek: store salt: %w", err)
	}
	return keystore.New(masterKey, salt)
}
