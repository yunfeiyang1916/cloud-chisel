package chclient

import (
	"context"
	"net"
	"net/http"
	"time"
)

// Config client的配置
type Config struct {
	// 指纹是通过使用对ECDSA公钥进行散列来生成的SHA256和base64编码的结果。
	// 指纹长度必须为44个字符，包含尾随的等号(=)
	// 对服务器的公钥执行主机密钥验证。指纹不匹配将关闭连接。
	Fingerprint string
	// 可选的用户名和密码(客户端身份验证),形式为:"<user>:<pass>"。
	// 将这些凭证与服务器的--authfile中的凭据进行比较。
	Auth string
	// 可选的保活间隔。 由于底层传输是HTTP，在许多情况下我们将遍历代理，这些代理通常会关闭空闲连接。
	// 您必须使用单位指定时间，例如“5s”或“2m”。 默认为“25s”（设置为 0s 以禁用）。
	KeepAlive time.Duration
	// 退出前重试的最大次数。 默认为无限制。
	MaxRetryCount int
	// 断开连接后重试前的最大等待时间。默认为 5 分钟。
	MaxRetryInterval time.Duration
	// chisel server url
	Server string
	// 一个可选的 HTTP CONNECT 或 SOCKS5 代理，将用于访问 chisel 服务器。
	// 可以在 URL 中指定身份验证,例如：
	// http://admin:password@my-server.com:8081
	// 或: socks://admin:password@my-server.com:1080
	Proxy string
	// 是通过服务器建立的远程连接隧道，
	// 每个连接都采用以下形式：<local-host>:<local-port>:<remote-host>:<remote-port>
	Remotes []string
	// Header 头，比如Foo: Bar
	Headers http.Header
	// 传输层安全协议的设置
	TLS TLSConfig
	// 拨号
	DialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

// TLSConfig Transport Layer Security 传输层安全协议的设置
type TLSConfig struct {
	// 跳过 TLS 证书验证（如果 TLS 用于与服务器的传输连接）。
	// 如果设置，客户端接受服务器提供的任何 TLS 证书以及该证书中的任何主机名。
	// 这只会影响传输 https (wss) 连接。 建立内部连接后，Chisel 服务器的公钥仍会被验证（参见 --fingerprint）。
	SkipVerify bool
	// 一个可选的根证书包，用于验证 chisel 服务器。仅在使用“https”或“wss”协议时有效。
	// 默认情况下，将使用操作系统 CA
	CA string
	// 与提供的私钥匹配的PEM编码证书的路径。该证书必须启用客户端身份验证
	Cert string
	// PEM编码的私钥路径，用于客户端身份验证
	Key string
}
