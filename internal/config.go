package internal

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/jinzhu/configor"
	log "github.com/sirupsen/logrus"
)

var Configuration = struct {
	Bot      BotConfiguration      `yaml:"bot"`
	Telegram TelegramConfiguration `yaml:"telegram"`
	Database DatabaseConfiguration `yaml:"database"`
	Lnbits   LnbitsConfiguration   `yaml:"lnbits"`
	Generate GenerateConfiguration `yaml:"generate"`
	Nostr    NostrConfiguration    `yaml:"nostr"`
	Pos      PosConfiguration      `yaml:"pos"`
	//Voucherbot VoucherbotConfiguration `yaml:"voucherbot"`
	Breez  BreezConfiguration  `yaml:"breez"`
	Limits LimitsConfiguration `yaml:"limits"`
}{}

type PosConfiguration struct {
	Currency    string `yaml:"currency"`
	Max_balance int64  `yaml:"max_balance"`
}

/*type VoucherbotConfiguration struct {
	Endpoint      string `yaml:"endpoint"`
	ApiKey        string `yaml:"api_key"`
	PurchaseType  string `yaml:"purchase_type"`
	DefaultAmount string `yaml:"default_amount"`
	Currency      string `yaml:"currency"`
}
*/

type NostrConfiguration struct {
	PrivateKey string `yaml:"private_key"`
}

type GenerateConfiguration struct {
	OpenAiBearerToken string `yaml:"open_ai_bearer_token"`
	DalleKey          string `yaml:"dalle_key"`
	DallePrice        int64  `yaml:"dalle_price"`
	Worker            int    `yaml:"worker"`
}

type SocksConfiguration struct {
	Host     string `yaml:"host"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type BotConfiguration struct {
	SocksProxy     *SocksConfiguration `yaml:"socks_proxy,omitempty"`
	TorProxy       *SocksConfiguration `yaml:"tor_proxy,omitempty"`
	LNURLServer    string              `yaml:"lnurl_server"`
	LNURLServerUrl *url.URL            `yaml:"-"`
	LNURLHostName  string              `yaml:"lnurl_public_host_name"`
	LNURLHostUrl   *url.URL            `yaml:"-"`
	LNURLSendImage bool                `yaml:"lnurl_image"`
	AdminAPIHost   string              `yaml:"admin_api_host"`
	Name           string              `yaml:"name"`
	Username       string              `yaml:"username"`
	Botadmin       string              `yaml:"botadmin"`
}

type TelegramConfiguration struct {
	MessageDisposeDuration int64  `yaml:"message_dispose_duration"`
	ApiKey                 string `yaml:"api_key"`
}

type DatabaseConfiguration struct {
	DbPath           string `yaml:"db_path"`
	ShopBuntDbPath   string `yaml:"shop_buntdb_path"`
	BuntDbPath       string `yaml:"buntdb_path"`
	TransactionsPath string `yaml:"transactions_path"`
	GroupsDbPath     string `yaml:"groupsdb_path"`
}

type LnbitsConfiguration struct {
	AdminId          string   `yaml:"admin_id"`
	AdminKey         string   `yaml:"admin_key"`
	Url              string   `yaml:"url"`
	LnbitsPublicUrl  string   `yaml:"lnbits_public_url"`
	WebhookServer    string   `yaml:"webhook_server"`
	WebhookCall      string   `yaml:"webhook_call"`
	WebhookServerUrl *url.URL `yaml:"-"`
}

type BreezConfiguration struct {
	Enabled           bool   `yaml:"enabled"`
	APIKey            string `yaml:"api_key"`
	WorkingDir        string `yaml:"working_dir"`
	Network           string `yaml:"network"`
	MnemonicEncrypted string `yaml:"mnemonic_encrypted"` // Deprecated - not used anymore
}

type LimitsConfiguration struct {
	LNbitsMaxBalance  int64 `yaml:"lnbits_max_balance"`  // S - Maximum balance to keep in LNbits
	AutoSwapThreshold int64 `yaml:"auto_swap_threshold"` // S2 - Trigger auto-swap when LNbits balance exceeds this
}

/*
func init() {
	err := configor.Load(&Configuration, "config.yaml")
	if err != nil {
		panic(err)
	}
	webhookUrl, err := url.Parse(Configuration.Lnbits.WebhookServer)
	if err != nil {
		panic(err)
	}
	Configuration.Lnbits.WebhookServerUrl = webhookUrl

	lnUrl, err := url.Parse(Configuration.Bot.LNURLServer)
	if err != nil {
		panic(err)
	}
	Configuration.Bot.LNURLServerUrl = lnUrl
	hostname, err := url.Parse(Configuration.Bot.LNURLHostName)
	if err != nil {
		panic(err)
	}
	Configuration.Bot.LNURLHostUrl = hostname
	checkLnbitsConfiguration()
	checkBreezConfiguration()
	checkLimitsConfiguration()
}
*/

// Init function to load configuration
func init() {
	err := configor.Load(&Configuration, "config.yaml")
	if err != nil {
		panic(err)
	}

	webhookUrl, err := url.Parse(Configuration.Lnbits.WebhookServer)
	if err != nil {
		panic(err)
	}
	Configuration.Lnbits.WebhookServerUrl = webhookUrl

	lnUrl, err := url.Parse(Configuration.Bot.LNURLServer)
	if err != nil {
		panic(err)
	}
	Configuration.Bot.LNURLServerUrl = lnUrl
	hostname, err := url.Parse(Configuration.Bot.LNURLHostName)
	if err != nil {
		panic(err)
	}
	Configuration.Bot.LNURLHostUrl = hostname
	checkLnbitsConfiguration()
	checkBreezConfiguration()
	checkLimitsConfiguration()
}

/*
func checkBreezConfiguration() {
	if Configuration.Breez.Enabled {
		if Configuration.Breez.WorkingDir == "" {
			Configuration.Breez.WorkingDir = "data/breez"
		}
		if Configuration.Breez.Network == "" {
			Configuration.Breez.Network = "testnet"
		}
		if Configuration.Breez.EncryptionKey == "" {
			panic(fmt.Errorf("breez.encryption_key is required when Breez is enabled (must be 64-char hex string)"))
		}
		// Validate encryption key is 64 hex characters (32 bytes)
		if len(Configuration.Breez.EncryptionKey) != 64 {
			panic(fmt.Errorf("breez.encryption_key must be exactly 64 hex characters (32 bytes), got %d characters", len(Configuration.Breez.EncryptionKey)))
		}
		log.Infof("[Config] Breez enabled: network=%s, workingDir=%s",
			Configuration.Breez.Network, Configuration.Breez.WorkingDir)
	} else {
		log.Debugf("[Config] Breez integration disabled")
	}
}
*/

// checkBreezConfiguration validates Breez settings
func checkBreezConfiguration() {
	if Configuration.Breez.Enabled {
		if Configuration.Breez.WorkingDir == "" {
			Configuration.Breez.WorkingDir = "data/breez"
		}
		if Configuration.Breez.Network == "" {
			Configuration.Breez.Network = "testnet"
		}

		log.Infof("[Config] Breez enabled: network=%s, workingDir=%s",
			Configuration.Breez.Network, Configuration.Breez.WorkingDir)
	} else {
		log.Debugf("[Config] Breez integration disabled")
	}
}

func checkLnbitsConfiguration() {
	if Configuration.Lnbits.Url == "" {
		panic(fmt.Errorf("please configure a lnbits url"))
	}
	if Configuration.Lnbits.LnbitsPublicUrl == "" {
		log.Warnf("Please specify a lnbits public url otherwise users won't be able to")
	} else {
		if !strings.HasSuffix(Configuration.Lnbits.LnbitsPublicUrl, "/") {
			Configuration.Lnbits.LnbitsPublicUrl = Configuration.Lnbits.LnbitsPublicUrl + "/"
		}
	}
}

func checkLimitsConfiguration() {
	// Validate required configuration
	if Configuration.Limits.LNbitsMaxBalance == 0 {
		panic(fmt.Errorf("please configure limits.lnbits_max_balance in config.yaml"))
	}
	if Configuration.Limits.AutoSwapThreshold == 0 {
		panic(fmt.Errorf("please configure limits.auto_swap_threshold in config.yaml"))
	}

	// Validate that auto-swap threshold is greater than max balance
	if Configuration.Limits.AutoSwapThreshold <= Configuration.Limits.LNbitsMaxBalance {
		panic(fmt.Errorf("limits.auto_swap_threshold (%d) must be greater than limits.lnbits_max_balance (%d)",
			Configuration.Limits.AutoSwapThreshold, Configuration.Limits.LNbitsMaxBalance))
	}

	log.Infof("[Config] Limits: LNbits max balance=%d sats, Auto-swap threshold=%d sats",
		Configuration.Limits.LNbitsMaxBalance, Configuration.Limits.AutoSwapThreshold)
}
