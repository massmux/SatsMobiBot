package telegram

import (
	"fmt"
	"time"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
)

func (bot *TipBot) backupHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	user := LoadUser(ctx)

	// Ensure private chat only
	if !m.Private() {
		bot.tryDeleteMessage(m)
		bot.trySendMessage(m.Sender, Translate(ctx, "backupPrivateOnlyMessage"))
		return ctx, errors.Create(errors.NoPrivateChatError)
	}

	// Get user's encrypted mnemonic from database
	if user.BreezMnemonic == "" {
		bot.trySendMessage(m.Sender, Translate(ctx, "backupNoMnemonicMessage"))
		return ctx, fmt.Errorf("no mnemonic stored for user")
	}

	// Try to decrypt the mnemonic (handles migration from plaintext)
	var mnemonic string
	mnemonic, err := breez.DecryptMnemonic(user.BreezMnemonic, internal.Configuration.Breez.EncryptionKey)
	if err != nil {
		// Migration: Check if mnemonic is plaintext (old format)
		log.Warnf("[/backup] Decryption failed, checking if plaintext mnemonic: %s", err)

		// Validate if it's a valid plaintext mnemonic
		if breez.ValidateMnemonic(user.BreezMnemonic) == nil {
			log.Infof("[/backup] Using plaintext mnemonic for user %s (will be migrated)", GetUserStr(user.Telegram))
			mnemonic = user.BreezMnemonic

			// Encrypt the plaintext mnemonic for future use
			encryptedMnemonic, encErr := breez.EncryptMnemonic(mnemonic, internal.Configuration.Breez.EncryptionKey)
			if encErr != nil {
				log.Errorf("[/backup] Failed to encrypt plaintext mnemonic: %s", encErr)
			} else {
				// Update DB with encrypted version
				user.BreezMnemonic = encryptedMnemonic
				if updateErr := UpdateUserRecord(user, *bot); updateErr != nil {
					log.Errorf("[/backup] Failed to save encrypted mnemonic: %s", updateErr)
				} else {
					log.Infof("[/backup] Successfully migrated and encrypted mnemonic for user %s", GetUserStr(user.Telegram))
				}
			}
		} else {
			log.Errorf("[/backup] Mnemonic is neither valid encrypted nor valid plaintext for user %s", GetUserStr(user.Telegram))
			bot.trySendMessage(m.Sender, Translate(ctx, "backupNoMnemonicMessage"))
			return ctx, fmt.Errorf("failed to decrypt mnemonic: %w", err)
		}
	}

	// Never reveal an invalid mnemonic (checksum)
	if valErr := breez.ValidateMnemonic(mnemonic); valErr != nil {
		log.Errorf("[/backup] Refusing to show invalid mnemonic to user %s: %s", GetUserStr(user.Telegram), valErr)
		bot.trySendMessage(m.Sender, Translate(ctx, "backupNoMnemonicMessage"))
		return ctx, fmt.Errorf("invalid mnemonic: %w", valErr)
	}

	// Send warning message first
	bot.trySendMessage(m.Sender, Translate(ctx, "backupWarningMessage"))
	time.Sleep(2 * time.Second)

	// Send the seed phrase
	backupMessage := fmt.Sprintf(Translate(ctx, "backupSeedMessage"), mnemonic)
	sentMsg := bot.trySendMessage(m.Sender, backupMessage)

	log.Infof("[/backup] User %s viewed their backup seed phrase", GetUserStr(user.Telegram))

	// Delete the message after 60 seconds
	time.AfterFunc(60*time.Second, func() {
		bot.tryDeleteMessage(sentMsg)
		bot.trySendMessage(m.Sender, Translate(ctx, "backupDeletedMessage"))
		log.Infof("[/backup] Seed phrase message deleted for user %s", GetUserStr(user.Telegram))
	})

	return ctx, nil
}
