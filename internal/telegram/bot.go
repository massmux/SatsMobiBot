package telegram

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/massmux/SatsMobiBot/internal/runtime/mutex"

	limiter "github.com/massmux/SatsMobiBot/internal/rate"

	"github.com/eko/gocache/store"

	"github.com/massmux/SatsMobiBot/internal"
	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/lnbits"
	"github.com/massmux/SatsMobiBot/internal/storage"
	gocache "github.com/patrickmn/go-cache"
	log "github.com/sirupsen/logrus"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

type TipBot struct {
	DB           *Databases
	Bunt         *storage.DB
	ShopBunt     *storage.DB
	Telegram     *tb.Bot
	Client       *lnbits.Client          // Custodial (LNbits)
	BreezClients map[int64]*breez.Client // Per-user Breez clients (userID -> client)
	breezMutex   sync.RWMutex            // Mutex for BreezClients map
	limiter      map[string]limiter.Limiter
	Cache
}
type Cache struct {
	*store.GoCacheStore
}

var (
	botWalletInitialisation     = sync.Once{}
	telegramHandlerRegistration = sync.Once{}
)

// NewBot migrates data and creates a new bot
func NewBot() TipBot {
	gocacheClient := gocache.New(5*time.Minute, 10*time.Minute)
	gocacheStore := store.NewGoCache(gocacheClient, nil)
	// create sqlite databases
	dbs := AutoMigration()
	limiter.Start()

	bot := TipBot{
		DB:           dbs,
		Client:       lnbits.NewClient(internal.Configuration.Lnbits.AdminKey, internal.Configuration.Lnbits.Url),
		Bunt:         createBunt(internal.Configuration.Database.BuntDbPath),
		ShopBunt:     createBunt(internal.Configuration.Database.ShopBuntDbPath),
		Telegram:     newTelegramBot(),
		Cache:        Cache{GoCacheStore: gocacheStore},
		BreezClients: make(map[int64]*breez.Client), // Initialize per-user Breez clients map
	}

	// Note: Breez is now initialized per-user when they run /start
	// No global Breez initialization needed

	return bot
}

// initializeUserBreezClient initializes a Breez SDK client for a specific user
func (bot *TipBot) initializeUserBreezClient(userID int64, mnemonic string) (*breez.Client, error) {
	if !internal.Configuration.Breez.Enabled {
		return nil, fmt.Errorf("Breez is not enabled in configuration")
	}

	// Create user-specific working directory
	userWorkingDir := fmt.Sprintf("%s/user_%d", internal.Configuration.Breez.WorkingDir, userID)

	config := &breez.Config{
		APIKey:     internal.Configuration.Breez.APIKey,
		WorkingDir: userWorkingDir,
		Network:    breez.Network(internal.Configuration.Breez.Network),
		Mnemonic:   mnemonic,
	}

	// Create client
	client, err := breez.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Breez client: %w", err)
	}

	// Initialize the client
	if err := client.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize Breez client: %w", err)
	}

	// Store in map
	bot.breezMutex.Lock()
	bot.BreezClients[userID] = client
	bot.breezMutex.Unlock()

	log.Infof("[Breez] User %d client initialized successfully", userID)
	return client, nil
}

// GetUserBreezClient returns the Breez client for a specific user
// NOTE: If user has PIN-encrypted mnemonic and client is not in memory,
// this will return nil and the calling function must request PIN from user
func (bot *TipBot) GetUserBreezClient(user *lnbits.User) *breez.Client {
	if user == nil || user.Telegram == nil {
		log.Debugf("[GetUserBreezClient] User or Telegram is nil")
		return nil
	}

	bot.breezMutex.RLock()
	client, exists := bot.BreezClients[user.Telegram.ID]
	bot.breezMutex.RUnlock()

	log.Debugf("[GetUserBreezClient] User ID %d - exists in map: %v, total clients: %d", user.Telegram.ID, exists, len(bot.BreezClients))

	if !exists {
		// Try to initialize if user has Breez enabled but client not in map
		if internal.Configuration.Breez.Enabled && user.BreezInitialized && user.BreezMnemonic != "" {
			log.Infof("[GetUserBreezClient] Breez client not in memory for user %d", user.Telegram.ID)

			// User has PIN-encrypted mnemonic, cannot decrypt without PIN
			if user.HasPin() {
				log.Infof("[GetUserBreezClient] User %d has PIN-encrypted mnemonic, cannot decrypt without PIN", user.Telegram.ID)
				// Return nil - calling function MUST request PIN
				return nil
			}

			// This should never happen in the new system
			// All users must have PIN to use Breez
			log.Errorf("[GetUserBreezClient] User %d has Breez but no PIN - invalid state", user.Telegram.ID)
			return nil
		}
		return nil
	}

	return client
}

// ✨ NUOVA FUNZIONE: InitializeBreezClientWithPin inizializza il client Breez dopo verifica PIN
func (bot *TipBot) InitializeBreezClientWithPin(user *lnbits.User, pin string) (*breez.Client, error) {
	if !user.HasPin() {
		return nil, fmt.Errorf("user does not have PIN set")
	}

	// Decifra la mnemonic con il PIN
	mnemonic, err := breez.DecryptMnemonicWithPin(user.BreezMnemonic, pin, user.PinSalt)
	if err != nil {
		log.Errorf("[InitializeBreezClientWithPin] Failed to decrypt mnemonic: %s", err)
		return nil, fmt.Errorf("failed to decrypt mnemonic with PIN")
	}

	// Valida la mnemonic
	if valErr := breez.ValidateMnemonic(mnemonic); valErr != nil {
		log.Errorf("[InitializeBreezClientWithPin] Invalid mnemonic: %s", valErr)
		return nil, fmt.Errorf("invalid mnemonic")
	}

	// Inizializza il client Breez
	client, err := bot.initializeUserBreezClient(user.Telegram.ID, mnemonic)
	if err != nil {
		log.Errorf("[InitializeBreezClientWithPin] Failed to initialize: %s", err)
		return nil, err
	}

	log.Infof("[InitializeBreezClientWithPin] Successfully initialized Breez client for user %d", user.Telegram.ID)
	return client, nil
}

// newTelegramBot will create a new Telegram bot.
func newTelegramBot() *tb.Bot {
	tgb, err := tb.NewBot(tb.Settings{
		Token:     internal.Configuration.Telegram.ApiKey,
		Poller:    &tb.LongPoller{Timeout: 60 * time.Second},
		ParseMode: tb.ModeMarkdown,
		Verbose:   false,
	})
	if err != nil {
		panic(err)
	}
	return tgb
}

// initBotWallet will create / initialize the bot wallet
// todo -- may want to derive user wallets from this specific bot wallet (master wallet), since lnbits usermanager extension is able to do that.
func (bot TipBot) initBotWallet() error {
	botWalletInitialisation.Do(func() {
		_, err := bot.initWallet(bot.Telegram.Me)
		if err != nil {
			log.Errorln(fmt.Sprintf("[initBotWallet] Could not initialize bot wallet: %s", err.Error()))
			return
		}
	})
	return nil
}

// GracefulShutdown will gracefully shutdown the bot
// It will wait for all mutex locks to unlock before shutdown.
func (bot *TipBot) GracefulShutdown() {
	// Shutdown all user Breez clients
	bot.breezMutex.Lock()
	for userID, client := range bot.BreezClients {
		if client != nil && client.IsInitialized() {
			log.Infof("[Breez] Shutting down client for user %d...", userID)
			if err := client.Shutdown(); err != nil {
				log.Errorf("[Breez] Shutdown error for user %d: %s", userID, err)
			}
		}
	}
	bot.BreezClients = make(map[int64]*breez.Client)
	bot.breezMutex.Unlock()

	t := time.NewTicker(time.Second * 10)
	log.Infof("[shutdown] Graceful shutdown (timeout=10s).")
	for {
		select {
		case <-t.C:
			// timer expired
			log.Infof("[shutdown] Graceful shutdown timeout reached. Forcing shutdown.")
			return
		default:
			// check if all mutex locks are unlocked
			if mutex.IsEmpty() {
				log.Infof("[shutdown] Graceful shutdown successful.")
				return
			}
		}
		time.Sleep(time.Second)
		log.Tracef("[shutdown] Trying graceful shutdown...")
	}
}

// Start will initialize the Telegram bot and lnbits.
func (bot *TipBot) Start() {
	log.Infof("[Telegram] Authorized on account @%s", bot.Telegram.Me.Username)
	// initialize the bot wallet
	err := bot.initBotWallet()
	if err != nil {
		log.Errorf("Could not initialize bot wallet: %s", err.Error())
	}

	// register telegram handlers
	bot.registerTelegramHandlers()

	// download bot avatar once
	bot.downloadMyProfilePicture()

	// edit worker collects messages to edit and
	// periodically edits them
	bot.startEditWorker()

	// register callbacks for invoices
	initInvoiceEventCallbacks(bot)

	// register callbacks for user state changes
	initializeStateCallbackMessage(bot)

	// start the telegram bot
	go bot.Telegram.Start()

	go bot.restartPersistedTickets()
	// gracefully shutdown
	exit := make(chan os.Signal, 1) // we need to reserve to buffer size 1, so the notifier are not blocked
	// we need to catch SIGTERM and SIGSTOP
	signal.Notify(exit, os.Interrupt, syscall.SIGTERM, syscall.SIGSTOP)
	<-exit
	// gracefully shutdown
	bot.GracefulShutdown()
}
