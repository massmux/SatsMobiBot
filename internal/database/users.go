package database

import (
	"strconv"
	"strings"

	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"gorm.io/gorm"
)

func FindUser(database *gorm.DB, username string) (*lnbits.User, *gorm.DB) {
	// now check for the user
	user := &lnbits.User{}
	// check if "username" is actually the user ID
	tx := database
	if _, err := strconv.ParseInt(username, 10, 64); err == nil {
		// asume it's anon_id
		tx = database.Where("anon_id = ?", username).First(user)
	} else if strings.HasPrefix(username, "0x") {
		// asume it's anon_id_sha256
		tx = database.Where("anon_id_sha256 = ?", username).First(user)
	} else if strings.HasPrefix(username, "1x") {
		// asume it's uuid
		tx = database.Where("uuid = ?", username).First(user)
	} else {
		// assume it's a string @username
		tx = database.Where("telegram_username = ? COLLATE NOCASE", username).First(user)
	}
	return user, tx
}

func FindUserSettings(user *lnbits.User, settingsTx *gorm.DB) (*lnbits.User, error) {
	// tx := bot.DB.Users.Preload("Settings").First(user)
	tx := settingsTx.First(user)
	if tx.Error != nil {
		return user, tx.Error
	}
	if user.Settings == nil {
		user.Settings = &lnbits.Settings{ID: user.ID}
	}
	return user, nil
}
