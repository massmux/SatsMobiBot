package userpage

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram"
	"github.com/PuerkitoBio/goquery"
	"github.com/fiatjaf/go-lnurl"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

type Service struct {
	bot *telegram.TipBot
}

func New(b *telegram.TipBot) Service {
	return Service{
		bot: b,
	}
}

// const botImage = "https://avatars.githubusercontent.com/u/88730856?v=7"
const botImage = "https://github.com/massmux/SatsMobiBot/blob/b3c74ce4609430bdfab0cda61ead645b25b9c68e/resources/satsmobibot-logo.png?raw=true"

//go:embed static
var templates embed.FS
var userpage_tmpl = template.Must(template.ParseFS(templates, "static/userpage.html"))
var qr_tmpl = template.Must(template.ParseFS(templates, "static/webapp.html"))

var Client = &http.Client{
	Timeout: 10 * time.Second,
}

// thank you fiatjaf for this code
func (s Service) getTelegramUserPictureURL(username string) (string, error) {
	// with proxy:
	// client, err := s.network.GetHttpClient()
	// if err != nil {
	// 	return "", err
	// }
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get("https://t.me/" + username)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return "", err
	}

	url, ok := doc.Find(`meta[property="og:image"]`).First().Attr("content")
	if !ok {
		return "", errors.New("no image available for this user")
	}

	return url, nil
}

func (s Service) UserPageHandler(w http.ResponseWriter, r *http.Request) {
	// https://sats.mobi/@<username>
	username := strings.ToLower(mux.Vars(r)["username"])
	callback := fmt.Sprintf("%s/.well-known/lnurlp/%s", internal.Configuration.Bot.LNURLHostName, username)
	log.Infof("[UserPage] rendering page of %s", username)
	lnurlEncode, err := lnurl.LNURLEncode(callback)
	if err != nil {
		log.Errorln("[UserPage]", err)
		return
	}
	image, err := s.getTelegramUserPictureURL(username)
	if err != nil || image == "https://telegram.org/img/t_logo.png" {
		// replace the default image
		image = botImage
	}

	if err := userpage_tmpl.ExecuteTemplate(w, "userpage", struct {
		Username string
		Image    string
		LNURLPay string
	}{username, image, lnurlEncode}); err != nil {
		log.Errorf("failed to render template")
	}
}

func (s Service) UserWebAppHandler(w http.ResponseWriter, r *http.Request) {
	// https://sats.mobi/app/<username>
	username := strings.ToLower(mux.Vars(r)["username"])
	callback := fmt.Sprintf("%s/.well-known/lnurlp/%s", internal.Configuration.Bot.LNURLHostName, username)
	log.Infof("[UserPage] rendering webapp of %s", username)
	lnurlEncode, err := lnurl.LNURLEncode(callback)
	if err != nil {
		log.Errorln("[UserPage]", err)
		return
	}
	if err := qr_tmpl.ExecuteTemplate(w, "webapp", struct {
		Username string
		LNURLPay string
		Callback string
	}{username, lnurlEncode, callback}); err != nil {
		log.Errorf("failed to render template")
	}
}
