package network

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

func GetClient() (*http.Client, error) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}
	if internal.Configuration.Bot.SocksProxy != nil {
		proxyURL, _ := url.Parse(internal.Configuration.Bot.SocksProxy.Host)
		specialTransport := &http.Transport{}
		specialTransport.Proxy = http.ProxyURL(proxyURL)
		var auth *proxy.Auth
		if internal.Configuration.Bot.SocksProxy.Username != "" && internal.Configuration.Bot.SocksProxy.Password != "" {
			auth = &proxy.Auth{User: internal.Configuration.Bot.SocksProxy.Username, Password: internal.Configuration.Bot.SocksProxy.Password}
		}
		d, err := proxy.SOCKS5("tcp", internal.Configuration.Bot.SocksProxy.Host, auth, &net.Dialer{
			Timeout:   20 * time.Second,
			Deadline:  time.Now().Add(time.Second * 10),
			KeepAlive: -1,
		})
		if err != nil {
			log.Errorln(err)
			return &client, nil
		}
		specialTransport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return d.Dial(network, addr)
		}
		client.Transport = specialTransport
	}
	return &client, nil
}
