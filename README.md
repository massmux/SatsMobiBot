# @SatsMobiBot

A Telegram Lightning ⚡️ Bitcoin wallet.

This repository contains everything you need to set up and run your own Tip bot and POS facility. If you simply want to use this bot in your group chat without having to install anything just start a conversation with [@SatsMobiBot](https://t.me/SatsMobiBot) and invite it into your group chat.

The system automatically creates a POS facility connected to your user. Getting payments in Lightning is immediate and requires no additional software installed and no externa APPs.

The system now provides also Scrub service. This service can be activated and deactivated realtime. This makes possible to automatic forward all incoming payments to an external Lightning Address. You can always change this address whenever you want or disable the service at all.

## Setting up the Bot

### Installation

This Bot adds features to the project it is forked from. It has been created to become a suite together to NFC Cards and other services connected. Being an open source project you are welcome to PR or to install yourself. This Bot by default is created with docker and runs a postgreSQL instance as database backend. I marked the Sqlite version as deprecated.


#### Create a Telegram bot

First, create a new Telegram bot by starting a conversation with the [@BotFather](https://core.telegram.org/bots#6-botfather). After you have created your bot, you will get an **Api Token** which you need to add to `telegram_api_key` in config.yaml accordingly.

#### Set up LNbits

Thanks to my friend Calle

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

## Subprojects using it:

- [sats.mobi](https://www.satsmobi.com)
- [NEPAY](https://www.nepay.ch)
