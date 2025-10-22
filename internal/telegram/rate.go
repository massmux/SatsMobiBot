package telegram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/massmux/SatsMobiBot/internal/breez"
	"github.com/massmux/SatsMobiBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"
)

// exchangeRateHandler handles the /rate command for currency conversions
func (bot *TipBot) exchangeRateHandler(ctx intercept.Context) (intercept.Context, error) {
	// Get user and check if user has Breez available
	user := LoadUser(ctx)
	userBreez := bot.GetUserBreezClient(user)
	if userBreez == nil || !userBreez.IsInitialized() {
		bot.trySendMessage(ctx.Sender(), "⚠️ Exchange rate service is currently unavailable.\nPlease run /start to initialize your Breez wallet.")
		return ctx, fmt.Errorf("user's Breez not available")
	}

	// Parse command: /rate [amount] [currency]
	// Examples:
	//   /rate              -> shows rate for 1000 sats in EUR
	//   /rate 10000        -> shows rate for 10000 sats in EUR
	//   /rate 10000 USD    -> shows rate for 10000 sats in USD
	//   /rate EUR          -> shows rate for 1000 sats in EUR

	args := strings.Fields(ctx.Message().Text)

	var amount int64 = 1000 // Default: 1000 sats
	currency := "EUR"       // Default currency

	// Parse arguments
	if len(args) >= 2 {
		// Try to parse second argument as amount
		if parsed, err := strconv.ParseInt(args[1], 10, 64); err == nil {
			amount = parsed
		} else {
			// If not a number, treat as currency
			currency = strings.ToUpper(args[1])
		}
	}

	if len(args) >= 3 {
		// Third argument is currency
		currency = strings.ToUpper(args[2])
	}

	// Validate amount
	if amount <= 0 {
		bot.trySendMessage(ctx.Sender(), "❌ Amount must be greater than 0.")
		return ctx, fmt.Errorf("invalid amount: %d", amount)
	}

	// Validate currency
	if !breez.IsSupportedCurrency(currency) {
		supportedList := strings.Join(breez.GetSupportedCurrencies(), ", ")
		message := fmt.Sprintf(
			"❌ Unsupported currency: %s\n\nSupported currencies:\n%s",
			currency, supportedList,
		)
		bot.trySendMessage(ctx.Sender(), message)
		return ctx, fmt.Errorf("unsupported currency: %s", currency)
	}

	// Get conversion
	fiatAmount, err := userBreez.ConvertSatsToFiat(amount, currency)
	if err != nil {
		log.Errorf("[/rate] Error converting %d sats to %s: %s", amount, currency, err)
		bot.trySendMessage(ctx.Sender(), fmt.Sprintf("❌ Error fetching exchange rate: %s", err))
		return ctx, err
	}

	// Get exchange rate info
	rateInfo, err := userBreez.GetExchangeRateInfo(currency)
	if err != nil {
		log.Errorf("[/rate] Error fetching rate info for %s: %s", currency, err)
	}

	// Format message
	formattedAmount := breez.FormatFiatAmount(fiatAmount, currency)
	message := fmt.Sprintf(
		"💱 *Exchange Rate*\n\n"+
			"`%d` sats = %s\n\n"+
			"1 BTC = %s",
		amount,
		formattedAmount,
		breez.FormatFiatAmount(rateInfo.Rate, currency),
	)

	log.Infof("[/rate] User requested conversion: %d sats = %s", amount, formattedAmount)
	bot.trySendMessage(ctx.Sender(), message)
	return ctx, nil
}

// rateHelp returns help text for the /rate command
func (bot *TipBot) rateHelp() string {
	supportedCurrencies := strings.Join(breez.GetSupportedCurrencies(), ", ")
	return fmt.Sprintf(
		"💱 *Exchange Rate Command*\n\n"+
			"Usage:\n"+
			"`/rate` - Show 1000 sats in EUR\n"+
			"`/rate 10000` - Show 10000 sats in EUR\n"+
			"`/rate 10000 USD` - Show 10000 sats in USD\n"+
			"`/rate EUR` - Show 1000 sats in EUR\n\n"+
			"Supported currencies:\n%s",
		supportedCurrencies,
	)
}
