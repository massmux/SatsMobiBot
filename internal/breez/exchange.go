package breez

import (
	"fmt"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// Supported fiat currencies
var SupportedCurrencies = []string{"USD", "EUR", "GBP", "JPY", "CNY", "CHF", "CAD", "AUD"}

// GetExchangeRate returns the current exchange rate for BTC to the specified fiat currency
func (c *Client) GetExchangeRate(fiatCurrency string) (float64, error) {
	if !c.initialized {
		return 0, fmt.Errorf("client not initialized")
	}

	// Normalize currency to uppercase
	currency := strings.ToUpper(fiatCurrency)

	// Validate currency
	if !IsSupportedCurrency(currency) {
		return 0, fmt.Errorf("unsupported currency: %s (supported: %v)", currency, SupportedCurrencies)
	}

	log.Debugf("[Breez] Fetching exchange rate for %s", currency)

	// Fetch fiat rates from Breez SDK
	rates, ratesErr := c.sdk.FetchFiatRates()
	if ratesErr != nil {
		return 0, fmt.Errorf("failed to fetch exchange rates: %s", ratesErr.AsError().Error())
	}

	// Find rate for requested currency
	for _, rate := range rates {
		if rate.Coin == currency {
			log.Debugf("[Breez] Exchange rate %s: %.2f", currency, rate.Value)
			return rate.Value, nil
		}
	}

	return 0, fmt.Errorf("rate not found for currency: %s", currency)
}

// ConvertSatsToFiat converts satoshis to fiat currency
func (c *Client) ConvertSatsToFiat(sats int64, currency string) (float64, error) {
	if sats < 0 {
		return 0, fmt.Errorf("amount cannot be negative")
	}

	rate, err := c.GetExchangeRate(currency)
	if err != nil {
		return 0, err
	}

	// Convert sats to BTC (1 BTC = 100,000,000 sats)
	btcAmount := float64(sats) / 100_000_000.0

	// Convert BTC to fiat
	fiatAmount := btcAmount * rate

	log.Debugf("[Breez] Converted %d sats to %.2f %s", sats, fiatAmount, currency)
	return fiatAmount, nil
}

// ConvertFiatToSats converts fiat currency to satoshis
func (c *Client) ConvertFiatToSats(fiatAmount float64, currency string) (int64, error) {
	if fiatAmount < 0 {
		return 0, fmt.Errorf("amount cannot be negative")
	}

	rate, err := c.GetExchangeRate(currency)
	if err != nil {
		return 0, err
	}

	// Convert fiat to BTC
	btcAmount := fiatAmount / rate

	// Convert BTC to sats (1 BTC = 100,000,000 sats)
	sats := int64(btcAmount * 100_000_000.0)

	log.Debugf("[Breez] Converted %.2f %s to %d sats", fiatAmount, currency, sats)
	return sats, nil
}

// GetExchangeRateInfo returns detailed exchange rate information
func (c *Client) GetExchangeRateInfo(currency string) (*ExchangeRateInfo, error) {
	rate, err := c.GetExchangeRate(currency)
	if err != nil {
		return nil, err
	}

	info := &ExchangeRateInfo{
		Currency:  strings.ToUpper(currency),
		Rate:      rate,
		UpdatedAt: time.Now().Unix(),
	}

	return info, nil
}

// IsSupportedCurrency checks if a currency is supported
func IsSupportedCurrency(currency string) bool {
	normalized := strings.ToUpper(currency)
	for _, supported := range SupportedCurrencies {
		if supported == normalized {
			return true
		}
	}
	return false
}

// GetSupportedCurrencies returns a list of supported currencies
func GetSupportedCurrencies() []string {
	return SupportedCurrencies
}

// FormatFiatAmount formats a fiat amount with the appropriate currency symbol
func FormatFiatAmount(amount float64, currency string) string {
	currency = strings.ToUpper(currency)

	symbols := map[string]string{
		"USD": "$",
		"EUR": "€",
		"GBP": "£",
		"JPY": "¥",
		"CNY": "¥",
		"CHF": "CHF ",
		"CAD": "C$",
		"AUD": "A$",
	}

	symbol, exists := symbols[currency]
	if !exists {
		symbol = currency + " "
	}

	// Format based on currency
	if currency == "JPY" || currency == "CNY" {
		// No decimals for JPY and CNY
		return fmt.Sprintf("%s%.0f", symbol, amount)
	}

	return fmt.Sprintf("%s%.2f", symbol, amount)
}
