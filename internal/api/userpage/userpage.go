package userpage

import (
	"embed"
	"errors"
	"fmt"
	"html/template"
	"math/bits"
	"net/http"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/str"
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

//go:embed static
var templates embed.FS
var tmpl = template.Must(template.ParseFS(templates, "static/userpage.html"))

var Client = &http.Client{
	Timeout: 10 * time.Second,
}

// thank you fiatjaf for this code
func (s Service) getTelegramUserPictureURL(username string) (string, error) {
	// with proxy:
	// client, err := s.bot.GetHttpClient()
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
	// https://ln.tips/.well-known/lnurlp/<username>
	username := mux.Vars(r)["username"]
	callback := fmt.Sprintf("%s/.well-known/lnurlp/%s", internal.Configuration.Bot.LNURLHostName, username)
	log.Infof("[UserPage] rendering page of %s", username)
	lnurlEncode, err := lnurl.LNURLEncode(callback)
	if err != nil {
		log.Errorln("[UserPage]", err)
		return
	}
	image, err := s.getTelegramUserPictureURL(username)
	if err != nil {
		log.Errorln("[UserPage]", err)
		image = "https://telegram.org/img/t_logo.png"
	}

	// generate random color
	i := str.Int32Hash(username)
	rgb_start := fmt.Sprintf("%d, %d, %d", i%256, bits.RotateLeft32(i, 1)%256, bits.RotateLeft32(i, 2)%256)
	rgb_end := fmt.Sprintf("%d, %d, %d", bits.RotateLeft32(i, 3)%256, bits.RotateLeft32(i, 4)%256, bits.RotateLeft32(i, 5)%256)

	log.Infof(rgb_start, rgb_end)
	if err := tmpl.ExecuteTemplate(w, "userpage", struct {
		Username string
		Image    string
		LNURLPay string
		RGBStart string
		RGBEnd   string
	}{username, image, lnurlEncode, rgb_start, rgb_end}); err != nil {
		log.Errorf("failed to render template")
	}
}
