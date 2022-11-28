package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
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

// checks if strings contains http(s)://*.onion
var isOnion = regexp.MustCompile("^https?\\:\\/\\/[\\w\\-\\.]+\\.onion")

// GetClientForScheme returns correct client for url scheme.
// if tld is .onion, function will also return an onion client.
func GetClientForScheme(url *url.URL) (*http.Client, error) {
	if isOnion.FindString(url.String()) != "" {
		return GetClient(ClientTypeTor)
	}
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
	if cfg.Host == "" {
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
