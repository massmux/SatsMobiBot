package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram"
	"gorm.io/gorm"
	"net/http"
	"net/http/httputil"
	"strings"

	log "github.com/sirupsen/logrus"
)

func LoggingMiddleware(prefix string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Tracef("[%s] %s %s", prefix, r.Method, r.URL.Path)
		log.Tracef("[%s]\n%s", prefix, dump(r))
		r.BasicAuth()
		next.ServeHTTP(w, r)
	}
}

type AuthType string

const (
	AuthTypeBasic  AuthType = "Basic"
	AuthTypeBearer AuthType = "Bearer"
)

func AuthorizationMiddleware(database *gorm.DB, authType AuthType, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		// check if the user is banned
		if auth != "" {
			_, password, ok := parseAuth(authType, auth)
			if !ok {
				return
			}
			// first we make sure that the password is not already "banned_"
			if strings.Contains(password, "_") || strings.HasPrefix(password, "banned_") {
				log.Warnf("[AuthorizationMiddleware] Banned user %s. Not forwarding request", password)
				return
			}
			// then we check whether the "normal" password provided is in the database (it should be not if the user is banned)
			user := &lnbits.User{}
			tx := database.Where("wallet_adminkey = ? COLLATE NOCASE", password).First(user)
			if tx.Error != nil {
				tx = database.Where("wallet_inkey = ? COLLATE NOCASE", password).First(user)
				log.Warnf("[AuthorizationMiddleware] Could not get wallet admin key %s: %v", password, tx.Error)
				if tx.Error != nil {
					log.Warnf("[AuthorizationMiddleware] need admin key to pay invoice %s: %v", password, tx.Error)
					return
				}
				if r.URL.Path == "/api/v1/payinvoice" {
					log.Warnf("[AuthorizationMiddleware] need admin key to pay invoice %s: %v", password, tx.Error)
					return
				}
			}
			r.Context()
			log.Debugf("[AuthorizationMiddleware] User: %s", telegram.GetUserStr(user.Telegram))
			r = r.WithContext(context.WithValue(r.Context(), "user", user))
		}

		next.ServeHTTP(w, r)
	}
}

// parseAuth parses an HTTP Basic Authentication string.
// "Bearer QWxhZGRpbjpvcGVuIHNlc2FtZQ==" returns ("Aladdin", "open sesame", true).
func parseAuth(authType AuthType, auth string) (username, password string, ok bool) {
	parse := func(prefix string) (username, password string, ok bool) {
		// Case insensitive prefix match. See Issue 22736.
		if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
			return
		}
		c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
		if err != nil {
			return
		}
		cs := string(c)
		s := strings.IndexByte(cs, ':')
		if s < 0 {
			return
		}
		return cs[:s], cs[s+1:], true
	}
	return parse(fmt.Sprintf("%s ", authType))

}

func dump(r *http.Request) string {
	x, err := httputil.DumpRequest(r, true)
	if err != nil {
		return ""
	}
	return string(x)
}
