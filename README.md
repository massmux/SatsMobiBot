# @SatsMobiBot

A Telegram Lightning ⚡️ Bitcoin wallet, with built-in POS, Scrub function and NFC Cards management. This project is a fork and evolution of the decommissioned LightningTipBot. This is the selfcustodial version based on breez library using Boltz liquid swaps.

This repository contains everything you need to set up and run your own Bot and POS facility. If you simply want to use this bot in your group chat without having to install anything just start a conversation with [@SatsMobiBot](https://t.me/SatsMobiBot) and invite it into your group chat.

The system automatically creates a POS facility connected to your user. Getting payments in Lightning is immediate and requires no additional software installed and no externa APPs.

This is a selfcustodial version. This means that a mnemonic phrase is generated and encrypted using a PIN defined by the user. User must backup the phrase + the PIN otherwise funds will be lost forever. Nobody may be able to unlock your funds if you lose PIN or phrase. Beware

Please also be informed that the system uses Boltz to swap from liquid to Lightning and they charge a small fee. This is fee that just depends on the swap service, not on the service runner.

The first time you run /start command, you will immediately get a @sats.mobi Lightning address connected and ready to go.

## Made with

- [LNbits](https://github.com/lnbits/lnbits) – Free and open-source lightning-network wallet/accounts system.
- [telebot](https://github.com/tucnak/telebot) – A Telegram bot framework in Go.
- [gozxing](https://github.com/makiuchi-d/gozxing) – barcode image processing library in Go.
- [ln-decodepay](https://github.com/fiatjaf/ln-decodepay) – Lightning Network BOLT11 invoice decoder.
- [go-lnurl](https://github.com/fiatjaf/go-lnurl) - Helpers for building lnurl support into services.

## What this Bot can do

This is a Lightning Wallet into a Telegram Bot, but more functionalities have been added:

- Notifications of Cards activations
- Integrated full POS service
- POS Link generation for executing POS on an external device

This project has educational and research purposes.

You can either compile the software in Go or use the pre-built docker image available on docker hub and using the docker-compose.yml file.

