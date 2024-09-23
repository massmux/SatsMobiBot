package nostr

import (
	"fmt"
	"net/http"

	"github.com/massmux/SatsMobiBot/internal/api"
	db "github.com/massmux/SatsMobiBot/internal/database"
	"github.com/massmux/SatsMobiBot/internal/telegram"
	"github.com/prometheus/common/log"
	"gorm.io/gorm"
)

type Nostr struct {
	database *gorm.DB
	bot      *telegram.TipBot
}

func New(bot *telegram.TipBot) Nostr {
	return Nostr{
		database: bot.DB.Users,
		bot:      bot,
	}
}

func (n Nostr) Handle(writer http.ResponseWriter, request *http.Request) {
	username := request.FormValue("name")
	if username == "" {
		api.NotFoundHandler(writer, fmt.Errorf("[NostrNip05] Form value 'name' is not set"))
		return
	}
	user, tx := db.FindUser(n.database, username)
	if tx.Error == nil && user.Telegram != nil {
		user, err := db.FindUserSettings(user, n.bot.DB.Users.Preload("Settings"))
		if err != nil {
			log.Errorf("[NostrNip05] user settings not found")
			api.NotFoundHandler(writer, fmt.Errorf("user settings error"))
			return
		}
		if user.Settings.Nostr.PubKey != "" {
			data := []byte(fmt.Sprintf(`{
  "names":{
    "%s":"%s"
  }
}
`, username, user.Settings.Nostr.PubKey))
			_, err = writer.Write(data)
			if err != nil {
				log.Errorf("[NostrNip05] Failed responding to user %s", username)
			}
		}
	} else {
		log.Errorf("[NostrNip05] user not found")
		api.NotFoundHandler(writer, fmt.Errorf("user not found"))
	}
}
