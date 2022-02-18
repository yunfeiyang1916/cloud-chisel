package chserver

import "time"

// Config server配置
type Config struct {
	KeySeed   string
	AuthFile  string
	Auth      string
	Proxy     string
	Socks5    bool
	Reverse   bool
	KeepAlive time.Duration
	TLS       TLSConfig
}
