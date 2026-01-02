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

	// Check if user has encrypted mnemonic stored in DB
	if user.BreezMnemonic != "" {
		// Try to decrypt existing mnemonic
		log.Infof("[Breez] Decrypting existing mnemonic for user %s", GetUserStr(user.Telegram))
		mnemonic, err = breez.DecryptMnemonic(user.BreezMnemonic, internal.Configuration.Breez.EncryptionKey)
		if err != nil {
			// Migration: Check if mnemonic is plaintext (old format)
			log.Warnf("[Breez] Decryption failed, checking if plaintext mnemonic: %s", err)

			// Validate if it's a valid plaintext mnemonic
			if breez.ValidateMnemonic(user.BreezMnemonic) == nil {
				log.Infof("[Breez] Migrating plaintext mnemonic to encrypted for user %s", GetUserStr(user.Telegram))
				mnemonic = user.BreezMnemonic

				// Encrypt the plaintext mnemonic
				encryptedMnemonic, encErr := breez.EncryptMnemonic(mnemonic, internal.Configuration.Breez.EncryptionKey)
				if encErr != nil {
					log.Errorf("[Breez] Failed to encrypt plaintext mnemonic: %s", encErr)
				} else {
					// Update DB with encrypted version
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
			log.Infof("[Breez] Successfully decrypted mnemonic for user %s", GetUserStr(user.Telegram))
		}
	} else {
		// Generate new mnemonic for first-time user
		log.Infof("[Breez] Generating new mnemonic for user %s", GetUserStr(user.Telegram))

		// Show creation message
		creationMsg := bot.trySendMessage(user.Telegram, "🔐 Creating your non-custodial wallet...")

		// Generate mnemonic
		mnemonic, err = breez.GenerateMnemonic()
		if err != nil {
			bot.tryDeleteMessage(creationMsg)
			return fmt.Errorf("failed to generate mnemonic: %w", err)
		}

		// Validate generated mnemonic (checksum)
		if valErr := breez.ValidateMnemonic(mnemonic); valErr != nil {
			bot.tryDeleteMessage(creationMsg)
			return fmt.Errorf("generated mnemonic failed validation: %w", valErr)
		}

		// Encrypt the mnemonic
		encryptedMnemonic, err := breez.EncryptMnemonic(mnemonic, internal.Configuration.Breez.EncryptionKey)
		if err != nil {
			bot.tryDeleteMessage(creationMsg)
			return fmt.Errorf("failed to encrypt mnemonic: %w", err)
		}

		// Store encrypted mnemonic in DB
		user.BreezMnemonic = encryptedMnemonic
		err = UpdateUserRecord(user, *bot)
		if err != nil {
			bot.tryDeleteMessage(creationMsg)
			return fmt.Errorf("failed to save encrypted mnemonic: %w", err)
		}

		log.Infof("[Breez] Generated and encrypted new mnemonic for user %s", GetUserStr(user.Telegram))

		// Delete creation message after 3 seconds
		time.AfterFunc(3*time.Second, func() {
			bot.tryDeleteMessage(creationMsg)
		})
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
