package proxy

import (
	"golang.org/x/net/proxy"
)

type Socks5ProxyConfig struct {
	Socks5ProxyEnabled  bool   `json:"socks5_enabled"`
	Socks5ProxyAddress  string `json:"socks5_address"`
	Socks5ProxyUsername string `json:"socks5_username"`
	Socks5ProxyPassword string `json:"socks5_password"`
}

func (c *Socks5ProxyConfig) GetDialer() (proxy.Dialer, error) {
	var auth *proxy.Auth
	if c.Socks5ProxyPassword != "" || c.Socks5ProxyUsername != "" {
		auth = new(proxy.Auth)
		auth.User = c.Socks5ProxyUsername
		auth.Password = c.Socks5ProxyPassword
	}
	return proxy.SOCKS5("tcp", c.Socks5ProxyAddress, auth, proxy.Direct)
}
