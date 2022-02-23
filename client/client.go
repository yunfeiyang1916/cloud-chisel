package chclient

import (
	"context"
	"crypto/md5"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	chshare "github.com/yunfeiyang1916/cloud-chisel/share"
	"github.com/yunfeiyang1916/cloud-chisel/share/ccrypto"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"github.com/yunfeiyang1916/cloud-chisel/share/tunnel"
	"golang.org/x/net/proxy"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
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
	// 退出前重试的最大次数。 -1表示无限制。
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
	// 一个可选的根证书路径，用于验证 chisel 服务器。仅在使用“https”或“wss”协议时有效。
	// 默认情况下，将使用操作系统 CA
	CA string
	// 与提供的私钥匹配的PEM编码证书的路径。该证书必须启用客户端身份验证
	Cert string
	// PEM编码的私钥路径，用于客户端身份验证
	Key string
}

// Client 客户端
type Client struct {
	*cio.Logger
	// 配置
	config *Config
	// 已计算好本地与远程服务的映射集合
	computed settings.Config
	// ssh 配置
	sshConfig *ssh.ClientConfig
	// 传输层安全协议的设置
	tlsConfig *tls.Config
	// 代理url
	proxyURL *url.URL
	// chisel server url
	server string
	// 连接计数器
	connCount cnet.ConnCount
	stop      func()
	eg        *errgroup.Group
	// ssh隧道
	tunnel *tunnel.Tunnel
}

func NewClient(c *Config) (*Client, error) {
	if !strings.HasPrefix(c.Server, "http") {
		c.Server = "http://" + c.Server
	}
	if c.MaxRetryInterval < time.Second {
		c.MaxRetryInterval = 5 * time.Minute
	}
	u, err := url.Parse(c.Server)
	if err != nil {
		return nil, err
	}
	// 将http替换成ws协议
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	if !regexp.MustCompile(`:\d+$`).MatchString(u.Host) {
		if u.Scheme == "wss" {
			u.Host = u.Host + ":443"
		} else {
			u.Host = u.Host + ":80"
		}
	}
	hasReverse := false
	hasSocks := false
	hasStdio := false
	client := &Client{
		Logger:    cio.NewLogger("client"),
		config:    c,
		computed:  settings.Config{Version: chshare.BuildVersion},
		server:    u.String(),
		tlsConfig: nil,
	}
	// 设置默认日志级别
	client.Logger.Info = true
	// 设置tls
	if u.Scheme == "wss" {
		tc := &tls.Config{}
		// 证书验证配置
		if c.TLS.SkipVerify {
			client.Infof("TLS verification disabled")
			tc.InsecureSkipVerify = true
		} else if c.TLS.CA != "" {
			rootCAs := x509.NewCertPool()
			if b, err := ioutil.ReadFile(c.TLS.CA); err != nil {
				return nil, fmt.Errorf("Failed to load file: %s", c.TLS.CA)
			} else if ok := rootCAs.AppendCertsFromPEM(b); !ok {
				return nil, fmt.Errorf("Failed to decode PEM: %s", c.TLS.CA)
			} else {
				client.Infof("TLS verification using CA %s", c.TLS.CA)
				tc.RootCAs = rootCAs
			}
		}
		// 提供客户端证书和密钥对
		if c.TLS.Cert != "" && c.TLS.Key != "" {
			c, err := tls.LoadX509KeyPair(c.TLS.Cert, c.TLS.Key)
			if err != nil {
				return nil, fmt.Errorf("Error loading client cert and key pair: %v", err)
			}
			tc.Certificates = []tls.Certificate{c}
		} else if c.TLS.Cert != "" || c.TLS.Key != "" {
			return nil, fmt.Errorf("Please specify client BOTH cert and key")
		}
		client.tlsConfig = tc
	}
	// 校验远程映射
	for _, s := range c.Remotes {
		r, err := settings.DecodeRemote(s)
		if err != nil {
			return nil, fmt.Errorf("Failed to decode remote '%s': %s", s, err)
		}
		if r.Socks {
			hasSocks = true
		}
		if r.Reverse {
			hasReverse = true
		}
		if r.Stdio {
			if hasStdio {
				return nil, errors.New("Only one stdio is allowed")
			}
			hasStdio = true
		}
		// 确认无反向隧道可用
		if !r.Reverse && !r.Stdio && !r.CanListen() {
			return nil, fmt.Errorf("Client cannot listen on %s", r.String())
		}
		client.computed.Remotes = append(client.computed.Remotes, r)
	}

	// 出站代理
	if p := c.Proxy; p != "" {
		client.proxyURL, err = url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("Invalid proxy URL (%s)", err)
		}
	}
	// ssh 认证和配置
	user, pass := settings.ParseAuth(c.Auth)
	client.sshConfig = &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		ClientVersion:   "SSH-" + chshare.ProtocolVersion + "-client",
		HostKeyCallback: client.verifyServer,
		Timeout:         settings.EnvDuration("SSH_TIMEOUT", 30*time.Second),
	}
	// 准备客户端隧道
	client.tunnel = tunnel.New(tunnel.Config{
		Logger:    client.Logger,
		Inbound:   true, // 客户端始终接受入站
		Outbound:  hasReverse,
		Socks:     hasReverse && hasSocks,
		KeepAlive: client.config.KeepAlive,
	})
	return client, nil
}

// Run 启动客户端并阻塞
func (c *Client) Run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := c.Start(ctx); err != nil {
		return err
	}
	return c.Wait()
}

// 验证服务器
func (c *Client) verifyServer(hostname string, remote net.Addr, key ssh.PublicKey) error {
	expect := c.config.Fingerprint
	if expect == "" {
		return nil
	}
	got := ccrypto.FingerprintKey(key)
	_, err := base64.StdEncoding.DecodeString(expect)
	if _, ok := err.(base64.CorruptInputError); ok {
		c.Logger.Infof("Specified deprecated MD5 fingerprint (%s), please update to the new SHA256 fingerprint: %s", expect, got)
		return c.verifyLegacyFingerprint(key)
	} else if err != nil {
		return fmt.Errorf("Error decoding fingerprint: %w", err)
	}
	if got != expect {
		return fmt.Errorf("Invalid fingerprint (%s)", got)
	}
	//overwrite with complete fingerprint
	c.Infof("Fingerprint %s", got)
	return nil
}

// verifyLegacyFingerprint 计算和比较传统MD5指纹
func (c *Client) verifyLegacyFingerprint(key ssh.PublicKey) error {
	bytes := md5.Sum(key.Marshal())
	strbytes := make([]string, len(bytes))
	for i, b := range bytes {
		strbytes[i] = fmt.Sprintf("%02x", b)
	}
	got := strings.Join(strbytes, ":")
	expect := c.config.Fingerprint
	if !strings.HasPrefix(got, expect) {
		return fmt.Errorf("Invalid fingerprint (%s)", got)
	}
	return nil
}

func (c *Client) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	c.stop = cancel
	eg, ctx := errgroup.WithContext(ctx)
	c.eg = eg
	via := ""
	if c.proxyURL != nil {
		via = " via " + c.proxyURL.String()
	}
	c.Infof("Connecting to %s%s\n", c.server, via)
	// 连接到 chisel server
	eg.Go(func() error {
		return c.connectionLoop(ctx)
	})
	// listen sockets
	eg.Go(func() error {
		clientInbound := c.computed.Remotes.Reversed(false)
		if len(clientInbound) == 0 {
			return nil
		}
		return c.tunnel.BindRemotes(ctx, clientInbound)
	})
	return nil
}

// 设置代理
func (c *Client) setProxy(u *url.URL, d *websocket.Dialer) error {
	// 连接代理,非socks代理
	if !strings.HasPrefix(u.Scheme, "socks") {
		d.Proxy = func(*http.Request) (*url.URL, error) {
			return u, nil
		}
		return nil
	}
	// SOCKS5 代理
	if u.Scheme != "socks" && u.Scheme != "socks5h" {
		return fmt.Errorf("unsupported socks proxy type: %s:// (only socks5h:// or socks:// is supported)", u.Scheme)
	}
	var auth *proxy.Auth
	if u.User != nil {
		pass, _ := u.User.Password()
		auth = &proxy.Auth{
			User:     u.User.Username(),
			Password: pass,
		}
	}
	socksDialer, err := proxy.SOCKS5("tcp", u.Host, auth, proxy.Direct)
	if err != nil {
		return err
	}
	d.NetDial = socksDialer.Dial
	return nil
}

// Wait blocks while the client is running.
func (c *Client) Wait() error {
	return c.eg.Wait()
}

// Close 关闭客户端
func (c *Client) Close() error {
	if c.stop != nil {
		c.stop()
	}
	return nil
}
