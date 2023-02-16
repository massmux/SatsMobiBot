package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	log "github.com/sirupsen/logrus"
)

var (
	nosterRegisterMessage       = "📖 Add your nostr pubkey for zap receipts"
	nostrHelpMessage            = "⚙️ *Commands:*\n`/nostr add <pubkey>` ✅ Add your nostr pubkeyt.\n`/nostr help` 📖 Show help."
	nostrAddedMessage           = "✅ *Nostr pubkey added.*"
	nostrPrivateKeyErrorMessage = "🚫 This is not your public key but your private key! Very dangerous! Try again with your npub..."
	nostrPublicKeyErrorMessage  = "🚫 There was an error decoding your public key."
)

func uniqueSlice(slice []string) []string {
	keys := make(map[string]bool)
	list := []string{}
	for _, entry := range slice {
		if _, value := keys[entry]; !value {
			keys[entry] = true
			list = append(list, entry)
		}
	}
	return list
}

func (bot *TipBot) publishNostrEvent(ev nostr.Event, relays []string) {
	pk := internal.Configuration.Nostr.PrivateKey

	// // BEGIN: testing
	// pub, _ := nostr.GetPublicKey(pk)
	// ev = nostr.Event{
	// 	PubKey:    pub,
	// 	CreatedAt: time.Now(),
	// 	Kind:      1,
	// 	Tags:      nil,
	// 	Content:   "Hello World!",
	// }
	// // END: testing

	// calling Sign sets the event ID field and the event Sig field
	ev.Sign(pk)
	log.Debugf("[NOSTR] publishing event %s", ev.ID)

	// more relays
	relays = append(relays, "wss://nostr.btcmp.com", "wss://nostr.relayer.se", "wss://relay.current.fyi", "wss://nos.lol", "wss://nostr.mom", "wss://relay.nostr.info", "wss://nostr.zebedee.cloud", "wss://nostr-pub.wellorder.net", "wss://relay.snort.social/", "wss://relay.damus.io/", "wss://nostr.oxtr.dev/", "wss://nostr.fmt.wiz.biz/")

	// publish the event to relays
	for _, url := range uniqueSlice(relays) {
		go func(url string) {
			relay, e := nostr.RelayConnect(context.Background(), url)
			if e != nil {
				log.Errorf(e.Error())
				return
			}
			status := relay.Publish(context.Background(), ev)
			log.Debugf("[NOSTR] published to %s: %s", url, status)
			relay.Close()
		}(url)

	}
}

func (bot *TipBot) nostrHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	splits := strings.Split(m.Text, " ")
	if len(splits) == 1 {
		return bot.getNostrHandler(ctx)
	} else if len(splits) > 1 {
		switch strings.ToLower(splits[1]) {
		case "add":
			return bot.addNostrPubkeyHandler(ctx)
		case "help":
			return bot.nostrHelpHandler(ctx)
		}
	}
	return ctx, nil
}

func (bot *TipBot) addNostrPubkeyHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	splits := strings.Split(m.Text, " ")
	splitlen := len(splits)
	if splitlen < 3 {
		return ctx, fmt.Errorf("not enough arguments")
	}
	nostrKeyInput := splits[2]

	// parse input
	if strings.HasPrefix(nostrKeyInput, "nsec") {
		bot.trySendMessage(ctx.Message().Sender, nostrPrivateKeyErrorMessage)
		return ctx, fmt.Errorf("user entered nostr private key")
	}
	// conver to hex
	if strings.HasPrefix(nostrKeyInput, "npub") {
		prefix, pubkey, err := nip19.Decode(nostrKeyInput)
		if err != nil {
			bot.trySendMessage(ctx.Message().Sender, nostrPublicKeyErrorMessage)
			return ctx, fmt.Errorf("shouldn't error: %s", err)
		}
		if prefix != "npub" {
			bot.trySendMessage(ctx.Message().Sender, nostrPublicKeyErrorMessage)
			return ctx, fmt.Errorf("returned invalid prefix")
		}
		nostrKeyInput = pubkey.(string)
	}

	user, err := GetLnbitsUserWithSettings(m.Sender, *bot)
	if err != nil {
		return ctx, err
	}
	// save node in db
	user.Settings.Nostr.PubKey = nostrKeyInput
	err = UpdateUserRecord(user, *bot)
	if err != nil {
		log.Errorf("[registerNodeHandler] could not update record of user %s: %v", GetUserStr(user.Telegram), err)
		return ctx, err
	}
	bot.trySendMessage(ctx.Message().Sender, nostrAddedMessage)
	return ctx, nil
}

func (bot *TipBot) nostrHelpHandler(ctx intercept.Context) (intercept.Context, error) {
	bot.trySendMessage(ctx.Message().Sender, nosterRegisterMessage+"\n\n"+nostrHelpMessage)
	return ctx, nil
}

func (bot *TipBot) getNostrHandler(ctx intercept.Context) (intercept.Context, error) {
	m := ctx.Message()
	user, err := GetLnbitsUserWithSettings(m.Sender, *bot)
	if err != nil {
		log.Infof("Could not get user settings for user %s", GetUserStr(user.Telegram))
		return ctx, err
	}

	if user.Settings == nil {
		bot.trySendMessage(m.Sender, nosterRegisterMessage+"\n\n"+nostrHelpMessage)
		return ctx, fmt.Errorf("no node registered")
	}

	node_info_str, err := nodeInfoString(&user.Settings.Node)
	if err != nil {
		log.Infof("Could not get node info for user %s", GetUserStr(user.Telegram))
		bot.trySendMessage(m.Sender, nosterRegisterMessage+"\n\n"+nostrHelpMessage)
		return ctx, err
	}
	bot.trySendMessage(m.Sender, node_info_str)

	return ctx, nil
}
