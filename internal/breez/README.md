# Breez SDK Integration

This package provides integration with the Breez SDK Liquid for self-custodial Lightning Network payments.

## Overview

The Breez integration is completely decoupled from the existing LNbits custodial wallet system. This allows for:

- **Dual Balance System**: Users can see both custodial (LNbits) and self-custodial (Breez) balances
- **Automatic Overflow**: Payments exceeding 100k sats in custodial wallet automatically route to Breez
- **Progressive Migration**: Foundation for eventual full migration from LNbits to Breez
- **Security**: Self-custodial model where users control their keys

## Architecture

### Package Structure

```
internal/breez/
├── client.go      # SDK initialization and connection lifecycle
├── types.go       # Breez-specific types, constants, and errors
├── wallet.go      # Wallet generation and connection functions
├── balance.go     # Balance retrieval operations
├── exchange.go    # Fiat exchange rate functions
├── config.go      # Configuration validation helpers
└── README.md      # This file
```

### Key Components

1. **Client**: Main wrapper around the Breez SDK
2. **Config**: Configuration management and validation
3. **Wallet**: Wallet generation and mnemonic management
4. **Balance**: Balance queries and tracking
5. **Exchange**: Fiat currency conversion

## Configuration

Add to `config.yaml`:

```yaml
breez:
  enabled: true
  api_key: "YOUR_BREEZ_API_KEY"
  working_dir: "data/breez"
  network: "testnet"  # or "mainnet"
  mnemonic_encrypted: ""  # Leave empty for new wallet generation
```

## Usage

### Initialization

The Breez client is automatically initialized when the bot starts if `breez.enabled: true` in config:

```go
// In internal/telegram/bot.go
bot := NewBot()  // Breez is initialized automatically
```

### Checking Balance

```go
// Get Breez balance
balance, err := bot.Breez.GetBalance()
if err != nil {
    log.Errorf("Failed to get Breez balance: %s", err)
}
```

### Exchange Rate Conversion

```go
// Convert 10,000 sats to EUR
fiatAmount, err := bot.Breez.ConvertSatsToFiat(10000, "EUR")
if err != nil {
    log.Errorf("Failed to convert: %s", err)
}

// Format with currency symbol
formatted := breez.FormatFiatAmount(fiatAmount, "EUR")  // "€420.00"
```

### Payment Routing

```go
// Decide where to route a payment
decision, err := bot.DecidePaymentRouting(user, incomingAmount)
if err != nil {
    // Handle error (e.g., Breez not available and would exceed limit)
}

if decision.RouteToBreez {
    // Create Breez invoice (Phase 2)
} else {
    // Create LNbits invoice
}
```

## Telegram Commands

### `/balance`
Shows both custodial (LNbits) and self-custodial (Breez) balances when Breez is enabled.

**Example output:**
```
💰 Your Balance

Custodial (LNbits): 50,000 sats
Self-Custodial (Breez): 150,000 sats
━━━━━━━━━━━━━━━━
Total: 200,000 sats
```

### `/rate [amount] [currency]`
Get BTC/fiat exchange rates and convert sats to fiat currency.

**Examples:**
- `/rate` - Show 1000 sats in EUR (default)
- `/rate 10000` - Show 10000 sats in EUR
- `/rate 10000 USD` - Show 10000 sats in USD
- `/rate EUR` - Show 1000 sats in EUR

**Supported currencies:** USD, EUR, GBP, JPY, CNY, CHF, CAD, AUD

## Phase 1 Implementation Status

### ✅ Completed

- [x] Breez SDK Liquid integration (v0.11.7)
- [x] Breez package structure
- [x] Client initialization and shutdown with real SDK
- [x] Configuration management
- [x] Wallet connection via mnemonic
- [x] Balance retrieval (real SDK calls)
- [x] Exchange rate functions (real-time rates from SDK)
- [x] Dual balance display
- [x] Payment routing logic
- [x] 100k custodial limit enforcement
- [x] `/rate` command for currency conversion
- [x] Integration with TipBot struct
- [x] Graceful shutdown handling with SDK disconnect

### 🚧 Phase 2 (Future)

- [ ] Invoice generation via Breez
- [ ] Payment receiving to Breez wallet
- [ ] LNURL-Pay for Breez wallet
- [ ] Per-user balance tracking in shared wallet
- [ ] Payment sending from Breez

### 🔮 Phase 3 (Future)

- [ ] Payment sending from Breez
- [ ] Internal transfers between Breez and LNbits
- [ ] User opt-in for Breez features
- [ ] Balance migration tools

### 🎯 Phase 4 (Future)

- [ ] Deprecate LNbits for new users
- [ ] Full migration path for existing users
- [ ] Breez-only mode

## Security Considerations

1. **Mnemonic Storage**: The mnemonic should be encrypted at rest (AES-256-GCM)
2. **Key Management**: Never log or expose mnemonics/private keys
3. **Access Control**: All Breez operations require proper user authentication
4. **Rate Limiting**: Exchange rate queries should be rate-limited
5. **Input Validation**: All amounts and currencies are validated
6. **Graceful Degradation**: LNbits continues working if Breez fails

## Testing

### Unit Tests

```bash
go test ./internal/breez/...
```

### Integration Tests

```bash
go test -tags=integration ./internal/breez/...
```

### Manual Testing

1. Start bot with `breez.enabled: false` - verify no errors
2. Start bot with `breez.enabled: true` - verify initialization
3. Test `/balance` command - verify dual balance display
4. Test `/rate` command - verify currency conversion
5. Test payment routing logic with different amounts
6. Test graceful shutdown

## Troubleshooting

### Breez fails to initialize

- Check `breez.api_key` is set correctly
- Verify `breez.working_dir` is writable
- Check network connectivity
- Review logs for specific error messages

### Exchange rates not working

- Ensure Breez is initialized
- Check supported currencies list
- Verify network connectivity (for real SDK)

### Balance shows 0

- If balance shows 0, ensure you have:
  - Provided a valid mnemonic in config
  - The wallet has been funded
  - The SDK has synced with the network (may take a moment on first connection)

## Development

### Adding New Features

1. Add function signature to `types.go` if needed
2. Implement function in appropriate file
3. Add error handling and logging
4. Update integration in `internal/telegram/`
5. Add tests
6. Update documentation

### Code Style

- Follow Go best practices
- Use descriptive variable names
- Add comments for exported functions
- Log important operations
- Handle all errors explicitly

## References

- [Breez SDK Documentation](https://sdk-doc.breez.technology/)
- [Breez SDK Liquid GitHub](https://github.com/breez/breez-sdk-liquid)
- [SatsMobiBot Architecture](../../ARCHITECTURE.md)
- [Integration Plan](../../BREEZ_INTEGRATION.md)

