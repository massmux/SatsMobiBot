# @SatsMobiBot

A Telegram Lightning ⚡️ Bitcoin wallet, with built-in POS, Scrub function and NFC Cards management. This project is a fork of LightningTipBot.

This repository contains everything you need to set up and run your own Tip bot and POS facility. If you simply want to use this bot in your group chat without having to install anything just start a conversation with [@SatsMobiBot](https://t.me/SatsMobiBot) and invite it into your group chat.

The system automatically creates a POS facility connected to your user. Getting payments in Lightning is immediate and requires no additional software installed and no externa APPs.

The system now provides also Scrub service. This service can be activated and deactivated realtime. This makes possible to automatic forward all incoming payments to an external Lightning Address. You can always change this address whenever you want or disable the service at all.

The first time you run /start command, you will immediately get a @sats.mobi Lightning address connected and ready to go.

## Made with

- [LNbits](https://github.com/lnbits/lnbits) – Free and open-source lightning-network wallet/accounts system.
- [telebot](https://github.com/tucnak/telebot) – A Telegram bot framework in Go.
- [gozxing](https://github.com/makiuchi-d/gozxing) – barcode image processing library in Go.
- [ln-decodepay](https://github.com/fiatjaf/ln-decodepay) – Lightning Network BOLT11 invoice decoder.
- [go-lnurl](https://github.com/fiatjaf/go-lnurl) - Helpers for building lnurl support into services.

## What this Bot can do

This is a Lightning Wallet into a Telegram Bot, but more functionalities have been added:

- /casback command to show a code to get a CashBack from a shop owner. In this case the amount is received and can be spent using the NFC Card connected to the Bot
- Activation of the NFC Card can be asked
- Notifications of Cards activations
- Integrated full POS service
- POS Link generation for executing POS on an external device
- Scrub service for forwarding all incoming payments to an external address, making the POS actually not custodial if activated

You can give the use of this Bot to your community. For example a physical shop manager can use this Bot + the NFC Cards + POS facility, all together. They can give the cards to their clients and send cashback for each purchase, thanks to the cashback command. The client will be able to spend the money just using his card everywhere.

