package telegram

import (
	"context"
	stderrors "errors"
	"fmt"
	"strconv"
	"time"

	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"

	"github.com/massmux/SatsMobiBot/internal/errors"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/breez"

	log "github.com/sirupsen/logrus"

	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/str"
	tb "gopkg.in/lightningtipbot/telebot.v3"
	"gorm.io/gorm"
)

func (bot TipBot) startHandler(ctx intercept.Context) (intercept.Context, error) {
	if !ctx.Message().Private() {
		return ctx, errors.Create(errors.NoPrivateChatError)
	}
	// ATTENTION: DO NOT CALL ANY HANDLER BEFORE THE WALLET IS CREATED
	// WILL RESULT IN AN ENDLESS LOOP OTHERWISE
	// bot.helpHandler(m)
	log.Printf("[⭐️ /start] New user: %s (%d)\n", GetUserStr(ctx.Sender()), ctx.Sender().ID)
	walletCreationMsg := bot.trySendMessageEditable(ctx.Sender(), Translate(ctx, "startSettingWalletMessage"))
	user, err := bot.initWallet(ctx.Sender())
	if err != nil {
		log.Errorln(fmt.Sprintf("[startHandler] Error with initWallet: %s", err.Error()))
		bot.tryEditMessage(walletCreationMsg, Translate(ctx, "startWalletErrorMessage"))
		return ctx, err
	}
	bot.tryDeleteMessage(walletCreationMsg)

	// Reload user from database to get the latest state (including BreezInitialized)
	user, err = GetUser(ctx.Sender(), bot)
	if err != nil {
		log.Errorln(fmt.Sprintf("[startHandler] Error reloading user: %s", err.Error()))
	}
	ctx.Context = context.WithValue(ctx, "user", user)

	// Sync Breez balance and check for refunds if Breez is initialized
	if user.BreezInitialized {
		userBreez := bot.GetUserBreezClient(user)
		if userBreez != nil && userBreez.IsInitialized() {
			log.Infof("[/start] Syncing Breez balance for user %s", GetUserStr(ctx.Sender()))

			// Sync balance with network
			if syncErr := userBreez.RefreshBalance(); syncErr != nil {
				log.Warnf("[/start] Failed to sync Breez balance: %s", syncErr)
			} else {
				log.Infof("[/start] Breez balance synced successfully")
			}

			// Check for pending refunds
			log.Infof("[/start] Checking for refundable swaps for user %s", GetUserStr(ctx.Sender()))
			if refundErr := bot.CheckAndNotifyRefunds(ctx, user); refundErr != nil {
				log.Warnf("[/start] Failed to check refunds: %s", refundErr)
			}
		}
	}

	// Check if user needs to set PIN before proceeding
	if internal.Configuration.Breez.Enabled && !user.HasPin() {
		log.Infof("[/start] User %s needs to set PIN, not showing help yet", GetUserStr(ctx.Sender()))
		// Don't show help or ready message yet - user needs to set PIN first
		return ctx, nil
	}

	bot.helpHandler(ctx)
	bot.trySendMessage(ctx.Sender(), Translate(ctx, "startWalletReadyMessage"))

	// Disable checking Balance when hitting Start
	//bot.balanceHandler(ctx)

	// send the user a warning about the need to set a username
	if len(ctx.Sender().Username) == 0 {
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "startNoUsernameMessage"), tb.NoPreview)
	}
	return ctx, nil
}

func (bot TipBot) initWallet(tguser *tb.User) (*lnbits.User, error) {
	user, err := GetUser(tguser, bot)
	if stderrors.Is(err, gorm.ErrRecordNotFound) {
		user = &lnbits.User{Telegram: tguser}
		err = bot.createWallet(user)
		if err != nil {
			return user, err
		}

		// Initialize Breez wallet for user if enabled
		if internal.Configuration.Breez.Enabled && !user.BreezInitialized {
			log.Infof("[initWallet] Initializing Breez wallet for user %s", GetUserStr(tguser))
			err = bot.initUserBreezWallet(user)
			if err != nil {
				log.Warnf("[initWallet] Failed to initialize Breez for user: %s", err)
				// Don't fail the whole initialization if Breez fails
			}
		}

		// set user initialized
		user, err := GetUser(tguser, bot)
		user.Initialized = true
		err = UpdateUserRecord(user, bot)
		if err != nil {
			log.Errorln(fmt.Sprintf("[initWallet] error updating user: %s", err.Error()))
			return user, err
		}
	} else if !user.Initialized {
		// update all tip tooltips (with the "initialize me" message) that this user might have received before
		tipTooltipInitializedHandler(user.Telegram, bot)
		user.Initialized = true

		// Initialize Breez if not already done
		if internal.Configuration.Breez.Enabled && !user.BreezInitialized {
			log.Infof("[initWallet] Initializing Breez wallet for existing user %s", GetUserStr(tguser))
			err = bot.initUserBreezWallet(user)
			if err != nil {
				log.Warnf("[initWallet] Failed to initialize Breez for user: %s", err)
			}
		}

		err = UpdateUserRecord(user, bot)
		if err != nil {
			log.Errorln(fmt.Sprintf("[initWallet] error updating user: %s", err.Error()))
			return user, err
		}
	} else if user.Initialized {
		// wallet is already initialized
		// Check if Breez needs to be initialized
		if internal.Configuration.Breez.Enabled && !user.BreezInitialized {
			log.Infof("[initWallet] Initializing Breez wallet for initialized user %s", GetUserStr(tguser))
			err = bot.initUserBreezWallet(user)
			if err != nil {
				log.Warnf("[initWallet] Failed to initialize Breez for user: %s", err)
			} else {
				// Update user record and clear cache to ensure fresh data
				err = UpdateUserRecord(user, bot)
				if err != nil {
					log.Warnf("[initWallet] Failed to update user record: %s", err)
				}
				// Clear cached user so next load gets fresh data
				bot.Cache.Delete(user.Name)
			}
		}
		return user, nil
	} else {
		err = fmt.Errorf("could not initialize wallet")
		return user, err
	}
	return user, nil
}

// initUserBreezWallet initializes Breez wallet for a user
func (bot *TipBot) initUserBreezWallet(user *lnbits.User) error {
	var mnemonic string
	var err error

	// ✨ NUOVO: Check if user has PIN set
	if !user.HasPin() {
		log.Infof("[Breez] User %s needs to set PIN first", GetUserStr(user.Telegram))

		msg := "🔐 *Welcome to Your Self-Custodial Wallet*\n\n" +
			"To protect your funds, you need to set a PIN.\n\n" +
			"This PIN will:\n" +
			"• Encrypt your seed phrase\n" +
			"• Protect your wallet access\n" +
			"• Cannot be recovered if forgotten\n\n" +
			"Use /setpin to continue."

		bot.trySendMessage(user.Telegram, msg, tb.ModeMarkdown)
		return fmt.Errorf("PIN required - user must set PIN first")
	}

	// Check if user has encrypted mnemonic stored in DB
	if user.BreezMnemonic != "" {
		// Try to decrypt existing mnemonic
		log.Infof("[Breez] User %s has existing encrypted mnemonic", GetUserStr(user.Telegram))

		// ✨ MODIFICATO: Prova prima con PIN, poi con master key (backward compatibility)
		if user.HasPin() {
			// L'utente ha un PIN, ma la mnemonic potrebbe essere ancora cifrata con master key
			// Proviamo prima con master key per backward compatibility
			mnemonic, err = breez.DecryptMnemonic(user.BreezMnemonic, internal.Configuration.Breez.EncryptionKey)
			if err != nil {
				// Se fallisce con master key, potrebbe essere già cifrata con PIN
				// Ma questo caso non dovrebbe verificarsi in questo flusso
				log.Warnf("[Breez] Failed to decrypt with master key: %s", err)

				// Check if it's plaintext
				if breez.ValidateMnemonic(user.BreezMnemonic) == nil {
					log.Infof("[Breez] Found plaintext mnemonic for user %s", GetUserStr(user.Telegram))
					mnemonic = user.BreezMnemonic

					// Encrypt with PIN
					encryptedMnemonic, encErr := breez.EncryptMnemonicWithPin(mnemonic, user.PinHash, user.PinSalt)
					if encErr != nil {
						log.Errorf("[Breez] Failed to encrypt with PIN: %s", encErr)
					} else {
						user.BreezMnemonic = encryptedMnemonic
						if updateErr := UpdateUserRecord(user, *bot); updateErr != nil {
							log.Errorf("[Breez] Failed to save PIN-encrypted mnemonic: %s", updateErr)
						} else {
							log.Infof("[Breez] Successfully migrated plaintext to PIN-encrypted mnemonic for user %s", GetUserStr(user.Telegram))
						}
					}
				} else {
					log.Errorf("[Breez] Mnemonic is neither valid encrypted nor valid plaintext")
					return fmt.Errorf("failed to decrypt mnemonic: %w", err)
				}
			} else {
				// Successfully decrypted with master key
				// Migrate to PIN encryption
				log.Infof("[Breez] Migrating from master key to PIN encryption for user %s", GetUserStr(user.Telegram))

				encryptedMnemonic, encErr := breez.EncryptMnemonicWithPin(mnemonic, user.PinHash, user.PinSalt)
				if encErr != nil {
					log.Errorf("[Breez] Failed to encrypt with PIN: %s", encErr)
					// Continue with master key encryption for now
				} else {
					user.BreezMnemonic = encryptedMnemonic
					if updateErr := UpdateUserRecord(user, *bot); updateErr != nil {
						log.Errorf("[Breez] Failed to save PIN-encrypted mnemonic: %s", updateErr)
					} else {
						log.Infof("[Breez] Successfully migrated to PIN encryption for user %s", GetUserStr(user.Telegram))
					}
				}
			}
		} else {
			// User doesn't have PIN yet, use master key (backward compatibility)
			mnemonic, err = breez.DecryptMnemonic(user.BreezMnemonic, internal.Configuration.Breez.EncryptionKey)
			if err != nil {
				// Migration: Check if mnemonic is plaintext (old format)
				log.Warnf("[Breez] Decryption failed, checking if plaintext mnemonic: %s", err)

				if breez.ValidateMnemonic(user.BreezMnemonic) == nil {
					log.Infof("[Breez] Using plaintext mnemonic for user %s", GetUserStr(user.Telegram))
					mnemonic = user.BreezMnemonic

					// Encrypt with master key
					encryptedMnemonic, encErr := breez.EncryptMnemonic(mnemonic, internal.Configuration.Breez.EncryptionKey)
					if encErr != nil {
						log.Errorf("[Breez] Failed to encrypt plaintext mnemonic: %s", encErr)
					} else {
						user.BreezMnemonic = encryptedMnemonic
						if updateErr := UpdateUserRecord(user, *bot); updateErr != nil {
							log.Errorf("[Breez] Failed to save encrypted mnemonic: %s", updateErr)
						} else {
							log.Infof("[Breez] Successfully migrated and encrypted mnemonic for user %s", GetUserStr(user.Telegram))
						}
					}
				} else {
					log.Errorf("[Breez] Mnemonic is neither valid encrypted nor valid plaintext")
					return fmt.Errorf("failed to decrypt mnemonic: %w", err)
				}
			} else {
				log.Infof("[Breez] Successfully decrypted mnemonic with master key for user %s", GetUserStr(user.Telegram))
			}
		}
	} else {
		// ✨ MODIFICATO: Generate new mnemonic with PIN protection
		log.Infof("[Breez] Generating new mnemonic for user %s", GetUserStr(user.Telegram))

		// Request PIN to encrypt the new mnemonic
		bot.trySendMessage(user.Telegram,
			"🔐 Enter your PIN to create your secure wallet:")

		user.StateKey = lnbits.UserStateEnterPinForOperation
		user.StateData = "create_wallet"
		UpdateUserRecord(user, *bot)

		return fmt.Errorf("waiting for PIN to create wallet")
	}

	// Final guardrail: never initialize Breez with an invalid mnemonic
	if valErr := breez.ValidateMnemonic(mnemonic); valErr != nil {
		log.Errorf("[Breez] Refusing to initialize Breez client with invalid mnemonic for user %s: %s", GetUserStr(user.Telegram), valErr)
		return fmt.Errorf("invalid mnemonic: %w", valErr)
	}

	// Initialize Breez client with decrypted mnemonic
	_, err = bot.initializeUserBreezClient(user.Telegram.ID, mnemonic)
	if err != nil {
		return fmt.Errorf("failed to initialize Breez client: %w", err)
	}

	user.BreezInitialized = true
	return nil
}

func (bot TipBot) createWallet(user *lnbits.User) error {
	UserStr := GetUserStr(user.Telegram)
	u, err := bot.Client.CreateUserWithInitialWallet(strconv.FormatInt(user.Telegram.ID, 10),
		fmt.Sprintf("%d (%s)", user.Telegram.ID, UserStr),
		internal.Configuration.Lnbits.AdminId,
		UserStr)
	if err != nil {
		errormsg := fmt.Sprintf("[createWallet] Create wallet error: %s", err.Error())
		log.Errorln(errormsg)
		return err
	}
	user.Wallet = &lnbits.Wallet{}
	user.ID = u.ID
	user.Name = u.Name
	wallet, err := bot.Client.Wallets(*user)
	if err != nil {
		errormsg := fmt.Sprintf("[createWallet] Get wallet error: %s", err.Error())
		log.Errorln(errormsg)
		return err
	}
	user.Wallet = &wallet[0]

	user.AnonID = fmt.Sprint(str.Int32Hash(user.ID))
	user.AnonIDSha256 = str.AnonIdSha256(user)
	user.UUID = str.UUIDSha256(user)

	user.Initialized = false
	user.CreatedAt = time.Now()
	err = UpdateUserRecord(user, bot)
	if err != nil {
		errormsg := fmt.Sprintf("[createWallet] Update user record error: %s", err.Error())
		log.Errorln(errormsg)
		return err
	}
	return nil
}
