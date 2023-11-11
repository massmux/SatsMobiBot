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
