package telegram

import (
	"fmt"
	"time"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
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

	// Check if user has PIN set
	if !user.HasPin() {
		bot.trySendMessage(m.Sender,
			"❌ You need to set a PIN first to secure your wallet.\n\n"+
				"Use /setpin to create your PIN.")
		return ctx, fmt.Errorf("no PIN set")
	}

	// Check if PIN is locked
	if user.IsPinLocked() {
		remaining := time.Until(*user.PinLockedUntil).Round(time.Minute)
		bot.trySendMessage(m.Sender,
			fmt.Sprintf("🔒 PIN locked due to too many failed attempts.\n\n"+
				"Try again in %v", remaining))
		return ctx, fmt.Errorf("PIN locked")
	}

	// Request PIN
	bot.trySendMessage(m.Sender,
		"🔐 Enter your PIN to view your seed phrase:")

	user.StateKey = lnbits.UserStateEnterPinForBackup
	user.StateData = "backup"
	UpdateUserRecord(user, *bot)

	return ctx, nil
}

// backupHandlerLegacy - Fallback per utenti che non hanno ancora impostato un PIN
// Usa la master key per backward compatibility
func (bot *TipBot) backupHandlerLegacy(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	user := LoadUser(ctx)

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

	log.Infof("[/backup] User %s viewed their backup seed phrase (legacy mode)", GetUserStr(user.Telegram))

	// Prompt user to set a PIN for better security
	time.Sleep(3 * time.Second)
	bot.trySendMessage(m.Sender,
		"⚠️ *Security Recommendation*\n\n"+
			"Set a PIN to better protect your wallet.\n"+
			"Use /setpin to get started.",
		)

	// Delete the message after 60 seconds
	time.AfterFunc(60*time.Second, func() {
		bot.tryDeleteMessage(sentMsg)
		bot.trySendMessage(m.Sender, Translate(ctx, "backupDeletedMessage"))
		log.Infof("[/backup] Seed phrase message deleted for user %s", GetUserStr(user.Telegram))
	})

	return ctx, nil
}
