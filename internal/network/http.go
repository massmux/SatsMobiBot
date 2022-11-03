package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/LightningTipBot/LightningTipBot/internal"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/proxy"
)

type ClientType string

const (
	ClientTypeClearNet = "clearnet"
	ClientTypeTor      = "tor"
)

func GetClientForScheme(url *url.URL) (*http.Client, error) {
	switch url.Scheme {
	case "onion":
		return GetClient(ClientTypeTor)
	default:
		return GetClient(ClientTypeClearNet)
	}
}

func GetClient(clientType ClientType) (*http.Client, error) {
	client := http.Client{
		Timeout: 10 * time.Second,
	}
	var cfg *internal.SocksConfiguration
	switch clientType {
	case ClientTypeClearNet:
		cfg = internal.Configuration.Bot.SocksProxy
	case ClientTypeTor:
		cfg = internal.Configuration.Bot.TorProxy
	default:
		return nil, fmt.Errorf("[GetClient] invalid clientType")
	}
	if cfg == nil {
		return &client, nil
	}
	proxyURL, _ := url.Parse(cfg.Host)
	specialTransport := &http.Transport{}
	specialTransport.Proxy = http.ProxyURL(proxyURL)
	var auth *proxy.Auth
	if cfg.Username != "" && cfg.Password != "" {
		auth = &proxy.Auth{User: cfg.Username, Password: cfg.Password}
	}
	d, err := proxy.SOCKS5("tcp", cfg.Host, auth, &net.Dialer{
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
	return &client, nil
}
