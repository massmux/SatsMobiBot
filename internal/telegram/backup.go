package telegram

import (
	"fmt"
	"time"

	"github.com/massmux/SatsMobiBot/internal/errors"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
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
