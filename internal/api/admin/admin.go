package admin

import (
	"github.com/massmux/SatsMobiBot/internal/telegram"
)

type Service struct {
	bot *telegram.TipBot
}

func New(b *telegram.TipBot) Service {
	return Service{
		bot: b,
	}
}
