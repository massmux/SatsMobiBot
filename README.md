# @SatsMobiBot

A Telegram Lightning ⚡️ Bitcoin wallet, with built-in POS, Scrub function and NFC Cards management. This project is a fork and evolution of the decommissioned LightningTipBot. This is the selfcustodial version based on breez library using Boltz liquid swaps.

This repository contains everything you need to set up and run your own Bot and POS facility. If you simply want to use this bot in your group chat without having to install anything just start a conversation with [@SatsMobiBot](https://t.me/SatsMobiBot) and invite it into your group chat.

The system automatically creates a POS facility connected to your user. Getting payments in Lightning is immediate and requires no additional software installed and no externa APPs.

This is a version which implements a self custodial deploy with the below described limitation. This means that a mnemonic phrase is generated and encrypted using a PIN defined by the user. User must backup the phrase + the PIN otherwise funds will be lost forever. Nobody may be able to unlock your funds if you lose PIN or phrase. Beware

Please also be informed that the system uses Boltz to swap from liquid to Lightning and they charge a small fee. This is fee that just depends on the swap service, not on the service runner.

The first time you run /start command, you will immediately get a @sats.mobi Lightning address connected and ready to go.

## Made with

- [LNbits](https://github.com/lnbits/lnbits) – Free and open-source lightning-network wallet/accounts system.
- [telebot](https://github.com/tucnak/telebot) – A Telegram bot framework in Go.
- [gozxing](https://github.com/makiuchi-d/gozxing) – barcode image processing library in Go.
- [ln-decodepay](https://github.com/fiatjaf/ln-decodepay) – Lightning Network BOLT11 invoice decoder.
- [go-lnurl](https://github.com/fiatjaf/go-lnurl) - Helpers for building lnurl support into services.
- [breesdk](https://github.com/breez/breez-sdk-liquid) - BreezSDK Liquid.

## What this Bot can do

This is a Lightning Wallet into a Telegram Bot, but more functionalities have been added:

- Notifications of Cards activations
- Integrated full POS service
- POS Link generation for executing POS on an external device

This project has educational and research purposes.

You can either compile the software in Go or use the pre-built docker image available on docker hub and using the docker-compose.yml file.

## Security Model & Technical Flow

SatsMobiBot implements a **PIN-based encryption system** for self-custodial Breez wallets. This section describes exactly what happens when a user interacts with the bot, and what security guarantees are — and are not — provided.

---

### How PIN Protection Works

When a user sets up their self-custodial wallet, the Breez SDK libreary used and the following happens:

1. **Mnemonic generation** — A fresh BIP39 mnemonic (seed phrase) is generated for the user using Breez SDK
2. **Key derivation** — A 256-bit AES key is derived from the user's PIN using PBKDF2-SHA256 with a random per-user salt.
3. **Encryption** — The mnemonic is encrypted with AES-256-GCM using the derived key.
4. **Storage** — Only the encrypted mnemonic and the salt are stored in the database. The PIN itself is never stored.

When a user subsequently uses a wallet command:

1. The user submits their PIN via Telegram message.
2. The PIN arrives as plaintext in the Go bot process memory.
3. PBKDF2 re-derives the AES key from the PIN and the stored salt.
4. The mnemonic is decrypted in RAM using AES-256-GCM.
5. The Breez SDK is initialized with the mnemonic.
6. The mnemonic is used for the operation and is not persisted anywhere in plaintext.

### Known Limitations

In the strict cryptographic sense. The following limitations apply:

| Risk | Description |
|---|---|
| **PIN in transit via Telegram** | The PIN travels as plaintext through Telegram's servers before reaching the bot. Telegram or a network-level adversary could intercept it. |
| **Mnemonic in RAM during session** | During an active wallet operation, the decrypted mnemonic exists in the Go process memory. An attacker with root access to the server could potentially extract it via memory inspection. |
| **Go memory management** | Go does not guarantee immediate zeroing of memory after use. The mnemonic may persist in RAM beyond its explicit scope until garbage collected. |


