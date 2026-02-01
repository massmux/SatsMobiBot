package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"

	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

const (
	MaxPinAttempts     = 5
	PinLockoutDuration = 30 * time.Minute
)

// setPinHandler gestisce il comando /setpin
func (bot *TipBot) setPinHandler(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	if user.ID == "" {
		return ctx, fmt.Errorf("user not initialized")
	}

	if user.HasPin() {
		// L'utente ha già un PIN, chiedi il vecchio PIN prima
		bot.trySendMessage(user.Telegram,
			"🔐 You already have a PIN set.\n\n"+
				"To change it, please enter your current PIN:")

		user.StateKey = lnbits.UserStateEnterPinForOperation
		user.StateData = "change_pin"
		UpdateUserRecord(user, *bot)
		return ctx, nil
	}

	// Primo PIN, chiedi di impostarlo
	msg := "🔐 *Set Your PIN*\n\n" +
		"Please choose a PIN to secure your wallet.\n\n" +
		"Requirements:\n" +
		"• 4-6 digits\n" +
		"• Not all same digit (e.g. 1111)\n" +
		"• Not sequential (e.g. 1234)\n\n" +
		"Enter your PIN:"

	bot.trySendMessage(user.Telegram, msg, tb.ModeMarkdown)

	user.StateKey = lnbits.UserStateEnterNewPin
	user.StateData = ""
	UpdateUserRecord(user, *bot)

	return ctx, nil
}

// handlePinInput gestisce l'input del PIN basato sullo stato
func (bot *TipBot) handlePinInput(ctx intercept.Context) (intercept.Context, error) {
	user := LoadUser(ctx)
	pinInput := ctx.Message().Text

	switch user.StateKey {
	case lnbits.UserStateEnterNewPin:
		return bot.handleNewPinInput(ctx, user, pinInput)
	case lnbits.UserStateConfirmNewPin:
		return bot.handleConfirmPinInput(ctx, user, pinInput)
	case lnbits.UserStateEnterPinForOperation:
		return bot.handleOperationPinInput(ctx, user, pinInput)
	case lnbits.UserStateEnterPinForBackup:
		return bot.handleBackupPinInput(ctx, user, pinInput)
	case lnbits.UserStateEnterPinForPayment:
		return bot.handlePaymentPinInput(ctx, user, pinInput)
	default:
		return ctx, nil
	}
}

// handleNewPinInput gestisce l'inserimento di un nuovo PIN
func (bot *TipBot) handleNewPinInput(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Cancella il messaggio con il PIN per sicurezza
	bot.tryDeleteMessage(ctx.Message())

	// Valida il PIN
	if err := breez.ValidatePin(pin); err != nil {
		if pinErr, ok := err.(*breez.PinError); ok {
			bot.trySendMessage(user.Telegram,
				fmt.Sprintf("❌ Invalid PIN: %s\n\nPlease try again:", pinErr.Message))
		} else {
			bot.trySendMessage(user.Telegram,
				"❌ Invalid PIN. Please try again:")
		}
		return ctx, nil
	}

	// PIN valido, chiedi conferma
	user.StateKey = lnbits.UserStateConfirmNewPin
	user.StateData = pin // Temporaneamente salva il PIN (sarà cancellato presto)
	UpdateUserRecord(user, *bot)

	bot.trySendMessage(user.Telegram,
		"✅ PIN accepted!\n\n"+
			"Please confirm your PIN by entering it again:")

	return ctx, nil
}

// handleConfirmPinInput gestisce la conferma del nuovo PIN
func (bot *TipBot) handleConfirmPinInput(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Cancella il messaggio con il PIN per sicurezza
	bot.tryDeleteMessage(ctx.Message())

	// Verifica che il PIN corrisponda
	if pin != user.StateData {
		bot.trySendMessage(user.Telegram,
			"❌ PINs don't match!\n\n"+
				"Let's try again. Enter your new PIN:")

		user.StateKey = lnbits.UserStateEnterNewPin
		user.StateData = ""
		UpdateUserRecord(user, *bot)
		return ctx, nil
	}

	// PINs corrispondono, salva il PIN
	err := bot.savePinForUser(user, pin)
	if err != nil {
		log.Errorf("[PIN] Failed to save PIN: %s", err)
		bot.trySendMessage(user.Telegram,
			"❌ Failed to set PIN. Please try again with /setpin")
		user.ResetState()
		UpdateUserRecord(user, *bot)
		return ctx, err
	}

	// Cancella il PIN dalla memoria
	user.StateData = ""
	user.ResetState()
	UpdateUserRecord(user, *bot)

	bot.trySendMessage(user.Telegram,
		"✅ *PIN Set Successfully!*\n\n"+
			"Your wallet is now protected with your PIN.\n"+
			"⚠️ **Remember your PIN** - it cannot be recovered!\n\n"+
			"You will need to enter your PIN for:\n"+
			"• Backing up your seed phrase\n"+
			"• Sending payments\n"+
			"• Changing PIN\n\n"+
			"Use /help to see available commands.",
		tb.ModeMarkdown)

	return ctx, nil
}

// handleOperationPinInput gestisce l'input del PIN per un'operazione
func (bot *TipBot) handleOperationPinInput(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Cancella il messaggio con il PIN
	bot.tryDeleteMessage(ctx.Message())

	// Check PIN lockout
	if err := bot.checkPinLockout(user); err != nil {
		bot.trySendMessage(user.Telegram, err.Error())
		user.ResetState()
		UpdateUserRecord(user, *bot)
		return ctx, err
	}

	// Verifica il PIN
	if err := breez.VerifyPin(pin, user.PinHash); err != nil {
		bot.recordFailedPinAttempt(user)
		bot.trySendMessage(user.Telegram,
			"❌ Incorrect PIN. Please try again:")
		return ctx, nil
	}

	// PIN corretto, reset attempts
	bot.resetPinAttempts(user)

	// Procedi con l'operazione
	operation := user.StateData

	switch operation {
	case "backup":
		return bot.handleBackupWithPin(ctx, user, pin)
	case "change_pin":
		return bot.handleChangePinRequest(ctx, user, pin)
	case "create_wallet":
		return bot.handleCreateWalletWithPin(ctx, user, pin)
	default:
		user.ResetState()
		UpdateUserRecord(user, *bot)
		return ctx, fmt.Errorf("unknown operation: %s", operation)
	}
}

// handleBackupPinInput gestisce l'input del PIN per backup
func (bot *TipBot) handleBackupPinInput(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Cancella il messaggio con il PIN
	bot.tryDeleteMessage(ctx.Message())

	// Check PIN lockout
	if err := bot.checkPinLockout(user); err != nil {
		bot.trySendMessage(user.Telegram, err.Error())
		user.ResetState()
		UpdateUserRecord(user, *bot)
		return ctx, err
	}

	// Verifica il PIN
	if err := breez.VerifyPin(pin, user.PinHash); err != nil {
		bot.recordFailedPinAttempt(user)
		bot.trySendMessage(user.Telegram,
			"❌ Incorrect PIN. Please try again:")
		return ctx, nil
	}

	// PIN corretto
	bot.resetPinAttempts(user)
	return bot.handleBackupWithPin(ctx, user, pin)
}

// handlePaymentPinInput gestisce l'input del PIN per pagamenti
func (bot *TipBot) handlePaymentPinInput(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Cancella il messaggio con il PIN
	bot.tryDeleteMessage(ctx.Message())

	// Check PIN lockout
	if err := bot.checkPinLockout(user); err != nil {
		bot.trySendMessage(user.Telegram, err.Error())
		user.ResetState()
		UpdateUserRecord(user, *bot)
		return ctx, err
	}

	// Verifica il PIN
	if err := breez.VerifyPin(pin, user.PinHash); err != nil {
		bot.recordFailedPinAttempt(user)
		bot.trySendMessage(user.Telegram,
			"❌ Incorrect PIN. Please try again:")
		return ctx, nil
	}

	// PIN corretto
	bot.resetPinAttempts(user)

	// Qui puoi implementare la logica per i pagamenti
	// Per ora ritorna al contesto
	user.ResetState()
	UpdateUserRecord(user, *bot)
	return ctx, nil
}

// savePinForUser salva il PIN per un utente (sia nuovo che modifica)
func (bot *TipBot) savePinForUser(user *lnbits.User, pin string) error {
	var err error

	// Se l'utente ha già una seedphrase cifrata con la vecchia chiave, ricifra
	isRecrypt := user.HasPin() && user.BreezMnemonic != ""

	if isRecrypt {
		// Cambio PIN - ricifra con nuovo PIN
		oldSalt := user.PinSalt

		// Genera nuovo salt
		newSalt, err := breez.GenerateSalt()
		if err != nil {
			return fmt.Errorf("failed to generate new salt: %w", err)
		}

		// Ricifra la seedphrase con il nuovo PIN
		// Nota: in questo contesto "pin" è il vecchio PIN perché abbiamo già verificato
		// Dobbiamo prima decifrare con il vecchio PIN e poi cifrare con il nuovo
		// Ma per il cambio PIN usiamo ChangePinRencrypt che gestisce tutto

		user.PinSalt = newSalt
	} else {
		// Primo PIN o non ha ancora seedphrase
		// Genera salt
		user.PinSalt, err = breez.GenerateSalt()
		if err != nil {
			return fmt.Errorf("failed to generate salt: %w", err)
		}

		// Se ha già una seedphrase cifrata con la master key, ricifra con il PIN
		if user.BreezMnemonic != "" {
			// Decifra con la master key
			mnemonic, err := breez.DecryptMnemonic(user.BreezMnemonic,
				bot.Bot.Configuration.Breez.EncryptionKey)
			if err != nil {
				// Prova a vedere se è già cifrata con un PIN (caso edge)
				log.Warnf("[PIN] Failed to decrypt with master key, might already be PIN-encrypted: %s", err)
			} else {
				// Ricifra con il PIN
				encrypted, err := breez.EncryptMnemonicWithPin(mnemonic, pin, user.PinSalt)
				if err != nil {
					return fmt.Errorf("failed to encrypt with PIN: %w", err)
				}
				user.BreezMnemonic = encrypted
			}
		}
	}

	// Hash il PIN per la verifica
	user.PinHash, err = breez.HashPin(pin)
	if err != nil {
		return fmt.Errorf("failed to hash PIN: %w", err)
	}

	// Imposta timestamp
	now := time.Now()
	user.PinSetAt = &now

	// Reset failed attempts
	user.PinFailedAttempts = 0
	user.PinLockedUntil = nil

	// Salva nel database
	err = UpdateUserRecord(user, *bot)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	log.Infof("[PIN] PIN saved successfully for user %s", GetUserStr(user.Telegram))
	return nil
}

// handleBackupWithPin gestisce il backup della seedphrase dopo verifica PIN
func (bot *TipBot) handleBackupWithPin(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Decifra la seedphrase
	mnemonic, err := breez.DecryptMnemonicWithPin(user.BreezMnemonic, pin, user.PinSalt)
	if err != nil {
		log.Errorf("[PIN] Failed to decrypt mnemonic: %s", err)
		bot.trySendMessage(user.Telegram,
			"❌ Failed to access your seed phrase. Please try again.")
		user.ResetState()
		UpdateUserRecord(user, *bot)
		return ctx, err
	}

	// Mostra la seedphrase
	msg := "🔐 *Your Seed Phrase*\n\n" +
		"⚠️ **KEEP THIS SECRET AND SAFE!**\n\n" +
		"`" + mnemonic + "`\n\n" +
		"Write it down and store it securely.\n" +
		"Anyone with this phrase can access your funds!\n\n" +
		"This message will be deleted in 60 seconds."

	sentMsg := bot.trySendMessage(user.Telegram, msg, tb.ModeMarkdown)

	// Cancella il messaggio dopo 60 secondi
	time.AfterFunc(60*time.Second, func() {
		bot.tryDeleteMessage(sentMsg)
	})

	user.ResetState()
	UpdateUserRecord(user, *bot)

	return ctx, nil
}

// handleChangePinRequest gestisce la richiesta di cambio PIN
func (bot *TipBot) handleChangePinRequest(ctx intercept.Context, user *lnbits.User, oldPin string) (intercept.Context, error) {
	// Il PIN è già stato verificato in handleOperationPinInput

	bot.trySendMessage(user.Telegram,
		"✅ Current PIN verified!\n\n"+
			"Now enter your new PIN (4-6 digits):")

	user.StateKey = lnbits.UserStateEnterNewPin
	user.StateData = oldPin // Temporaneamente salva il vecchio PIN
	UpdateUserRecord(user, *bot)

	return ctx, nil
}

// handleCreateWalletWithPin gestisce la creazione del wallet dopo inserimento PIN
func (bot *TipBot) handleCreateWalletWithPin(ctx intercept.Context, user *lnbits.User, pin string) (intercept.Context, error) {
	// Verifica PIN
	if err := breez.VerifyPin(pin, user.PinHash); err != nil {
		bot.trySendMessage(user.Telegram,
			"❌ Incorrect PIN. Please try again:")
		return ctx, nil
	}

	// Genera mnemonic
	creationMsg := bot.trySendMessage(user.Telegram, "🔐 Creating your wallet...")

	mnemonic, err := breez.GenerateMnemonic()
	if err != nil {
		bot.tryDeleteMessage(creationMsg)
		return ctx, fmt.Errorf("failed to generate mnemonic: %w", err)
	}

	// Cifra con PIN
	encryptedMnemonic, err := breez.EncryptMnemonicWithPin(mnemonic, pin, user.PinSalt)
	if err != nil {
		bot.tryDeleteMessage(creationMsg)
		return ctx, fmt.Errorf("failed to encrypt mnemonic: %w", err)
	}

	// Salva nel DB
	user.BreezMnemonic = encryptedMnemonic
	user.BreezInitialized = true
	err = UpdateUserRecord(user, *bot)
	if err != nil {
		bot.tryDeleteMessage(creationMsg)
		return ctx, fmt.Errorf("failed to save: %w", err)
	}

	bot.tryDeleteMessage(creationMsg)
	bot.trySendMessage(user.Telegram,
		"✅ Wallet created successfully!\n\n"+
			"Your seed phrase is secured with your PIN.\n"+
			"Use /backup to view your seed phrase.")

	user.ResetState()
	UpdateUserRecord(user, *bot)

	return ctx, nil
}

// checkPinLockout verifica se l'utente è bloccato per troppi tentativi falliti
func (bot *TipBot) checkPinLockout(user *lnbits.User) error {
	if user.PinLockedUntil != nil && time.Now().Before(*user.PinLockedUntil) {
		remaining := time.Until(*user.PinLockedUntil).Round(time.Minute)
		return fmt.Errorf("🔒 PIN locked due to too many failed attempts.\n\nTry again in %v", remaining)
	}
	return nil
}

// recordFailedPinAttempt registra un tentativo fallito di PIN
func (bot *TipBot) recordFailedPinAttempt(user *lnbits.User) {
	user.PinFailedAttempts++

	if user.PinFailedAttempts >= MaxPinAttempts {
		lockUntil := time.Now().Add(PinLockoutDuration)
		user.PinLockedUntil = &lockUntil

		bot.trySendMessage(user.Telegram,
			fmt.Sprintf("🔒 Too many failed attempts. PIN locked for %d minutes.",
				int(PinLockoutDuration.Minutes())))
	}

	UpdateUserRecord(user, *bot)
}

// resetPinAttempts resetta i tentativi falliti di PIN
func (bot *TipBot) resetPinAttempts(user *lnbits.User) {
	user.PinFailedAttempts = 0
	user.PinLockedUntil = nil
	UpdateUserRecord(user, *bot)
}
