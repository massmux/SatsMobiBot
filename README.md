# @SatsMobiBot

A Telegram Lightning ⚡️ Bitcoin wallet and tip bot for group chats.

This repository contains everything you need to set up and run your own tip bot. If you simply want to use this bot in your group chat without having to install anything just start a conversation with [@SatsMobiBot](https://t.me/SatsMobiBot) and invite it into your group chat.

## Setting up the Bot

### Installation

This Bot is a customized version of LightningTipBot, meaning that it has been modified for the needed tasks, so it may be more specific than the forked repo. For a general purpose Bot maybe better to refer to forked repo. A complete guide to install and run SatsMobiBot (LightningTipBot) + LNBITS (on docker with PostgreSQL) on the same VPS with an external LND funding source has been prepared by Massimo Musumeci (@massmux) and it is available: [LightningTipBot full install](https://www.massmux.com/howto-complete-lightningtipbot-lnbits-setup-vps/)


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

You can give the use of this Bot to your community. For example a physical shop manager can use this Bot + the NFC Cards. They can give the cards to their clients and send cashback for each purchase, thanks to the cashback command. The client will be able to spend the money just using his card everywhere.

## Subprojects using it:

- [sats.mobi](https://www.satsmobi.com)
- [NEPay](https://www.nepay.ch)
