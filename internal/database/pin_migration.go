package database

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// MigratePinFields adds PIN-related fields to the users table
func MigratePinFields(db *gorm.DB) error {
	log.Info("[Migration] Adding PIN fields to users table")

	// Define the User struct with only the new PIN fields
	type User struct {
		PinSalt           string     `gorm:"type:varchar(128)"`
		PinHash           string     `gorm:"type:varchar(128)"`
		PinSetAt          *time.Time `gorm:"type:datetime"`
		PinFailedAttempts int        `gorm:"default:0"`
		PinLockedUntil    *time.Time `gorm:"type:datetime"`
	}

	// Run auto-migration to add the new fields
	err := db.AutoMigrate(&User{})
	if err != nil {
		return fmt.Errorf("failed to migrate PIN fields: %w", err)
	}

	log.Info("[Migration] Successfully added PIN fields to users table")
	
	// Log statistics about existing users
	var totalUsers int64
	db.Table("users").Count(&totalUsers)
	
	var usersWithMnemonic int64
	db.Table("users").Where("breez_mnemonic != ''").Count(&usersWithMnemonic)
	
	log.Infof("[Migration] Database statistics:")
	log.Infof("  - Total users: %d", totalUsers)
	log.Infof("  - Users with mnemonic: %d", usersWithMnemonic)
	log.Infof("  - Users will be prompted to set PIN on next wallet access")
	
	return nil
}
