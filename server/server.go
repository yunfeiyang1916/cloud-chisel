package chserver

import (
	"context"
	"errors"
	"github.com/gorilla/websocket"
	"github.com/jpillora/requestlog"
	chshare "github.com/yunfeiyang1916/cloud-chisel/share"
	"github.com/yunfeiyang1916/cloud-chisel/share/ccrypto"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"github.com/yunfeiyang1916/cloud-chisel/share/tunnel"
	"golang.org/x/crypto/ssh"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"time"
)

// Config server配置
type Config struct {
	KeySeed string
	// 一个可选的user.json路径。这个文件是一个对象，如下定义：{"<user:pass>": ["<addr-regex>","<addr-regex>"]}
	// 当使用<user>连接时，<pass>将被验证，然后每个远程地址将与列表进行正则匹配
	// 普通远程地址形式：<remote-host>:<remote-port>
	// 用于反向端口转发远程地址形式：R:<local-interface>:<local-port>
	AuthFile string
	// 形式为：<user:pass>，可选。
	// 等价于authfile {"<user:pass>": [""]},如果未设置，则将使用AUTH环境变量
	Auth string
	// 代理
	Proxy string
	// 是否允许客户端访问内部的SOCKS5代理
	Socks5 bool
	// 反向端口转发，是否允许客户端指定反向端口转发
	Reverse bool
	// 可选的保活间隔。 由于底层传输是HTTP，在许多情况下我们将遍历代理，这些代理通常会关闭空闲连接。
	// 您必须使用单位指定时间，例如“5s”或“2m”。 默认为“25s”（设置为 0s 以禁用）。
	KeepAlive time.Duration
	// 传输层安全协议的设置
	TLS TLSConfig
	onConnect      func(localPort string, tun *tunnel.Tunnel)
	onConnectClose func(localPort string)
}

type Server struct {
	*cio.Logger
	config *Config
	// 认证指纹
	fingerprint string
	// 提供http服务
	httpServer *cnet.HTTPServer
	// 反向代理，接收传入的请求并将其发送到另一个服务器，将响应代理回客户端。
	// 默认情况下将客户端IP设置为X-Forwarded-For报头的值
	reverseProxy *httputil.ReverseProxy
	// session数量
	sessCount int32
	// 会话
	sessions  *settings.Users
	sshConfig *ssh.ServerConfig
	// 可重载的用户源配置
	users *settings.UserIndex
}

// 升级器，将http连接升级成websocket
var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  settings.EnvInt("WS_BUFF_SIZE", 0),
	WriteBufferSize: settings.EnvInt("WS_BUFF_SIZE", 0),
}

// NewServer 创建 chisel server
func NewServer(c *Config) (*Server, error) {
	server := &Server{
		config:     c,
		httpServer: cnet.NewHTTPServer(),
		Logger:     cio.NewLogger("server"),
		sessions:   settings.NewUsers(),
	}
	server.Info = true
	server.users = settings.NewUserIndex(server.Logger)
	if c.AuthFile != "" {
		if err := server.users.LoadUsers(c.AuthFile); err != nil {
			return nil, err
		}
	}
	if c.Auth != "" {
		u := &settings.User{Addrs: []*regexp.Regexp{settings.UserAllowAll}}
		u.Name, u.Pass = settings.ParseAuth(c.Auth)
		if u.Name != "" {
			server.users.AddUser(u)
		}
	}
	// 生成私钥(可选地使用种子)
	key, err := ccrypto.GenerateKey(c.KeySeed)
	// 转成ssh私钥
	private, err := ssh.ParsePrivateKey(key)
	if err != nil {
		log.Fatal("Failed to parse key")
	}
	// 生成指纹
	server.fingerprint = ccrypto.FingerprintKey(private.PublicKey())
	//create ssh config
	server.sshConfig = &ssh.ServerConfig{
		ServerVersion:    "SSH-" + chshare.ProtocolVersion + "-server",
		PasswordCallback: server.authUser,
	}
	server.sshConfig.AddHostKey(private)
	// 设置代理
	if c.Proxy != "" {
		u, err := url.Parse(c.Proxy)
		if err != nil {
			return nil, err
		}
		if u.Host == "" {
			return nil, server.Errorf("Missing protocol (%s)", u)
		}
		server.reverseProxy = httputil.NewSingleHostReverseProxy(u)
		// 始终使用代理主机
		server.reverseProxy.Director = func(r *http.Request) {
			r.URL.Scheme = u.Scheme
			r.URL.Host = u.Host
			r.Host = u.Host
		}
	}
	// 当反向隧道启用时打印日志
	if c.Reverse {
		server.Infof("Reverse tunnelling enabled")
	}
	return server, nil
}

// Run 运行服务，内部调用 Start 和 Wait.
func (s *Server) Run(host, port string) error {
	if err := s.Start(host, port); err != nil {
		return err
	}
	return s.Wait()
}

// Start 启动http服务器，不提供取消上下文
func (s *Server) Start(host, port string) error {
	return s.StartContext(context.Background(), host, port)
}

// StartContext 启动http服务器，可以通过取消提供的上下文来关闭
func (s *Server) StartContext(ctx context.Context, host, port string) error {
	s.Infof("Fingerprint %s", s.fingerprint)
	if s.users.Len() > 0 {
		s.Infof("User authenication enabled")
	}
	if s.reverseProxy != nil {
		s.Infof("Reverse proxy enabled")
	}
	l, err := s.listener(host, port)
	if err != nil {
		return err
	}
	h := http.Handler(http.HandlerFunc(s.handleClientHandler))
	if s.Debug {
		o := requestlog.DefaultOptions
		o.TrustProxy = true
		h = requestlog.WrapWith(h, o)
	}
	return s.httpServer.GoServer(ctx, l, h)
}

// Wait 等待http server关闭
func (s *Server) Wait() error {
	return s.httpServer.Wait()
}

// Close 强制关闭HTTP服务器
func (s *Server) Close() error {
	return s.httpServer.Close()
}

// GetFingerprint 获取访问服务器的指纹
func (s *Server) GetFingerprint() string {
	return s.fingerprint
}

// ssh验证用户名和密码
func (s *Server) authUser(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
	if s.users.Len() == 0 {
		return nil, nil
	}
	// 检查用户是否存在且密码匹配
	n := c.User()
	user, found := s.users.Get(n)
	if !found || user.Pass != string(password) {
		s.Debugf("Login failed for user: %s", n)
		return nil, errors.New("Invalid authentication for username: %s")
	}
	// insert the user session map
	// TODO 应该加个互斥锁
	s.sessions.Set(string(c.SessionID()), user)
	return nil, nil
}

// DeleteUser removes a user from the server user index
func (s *Server) DeleteUser(user string) {
	s.users.Del(user)
}

// ResetUsers in the server user index.
// Use nil to remove all.
func (s *Server) ResetUsers(users []*settings.User) {
	s.users.Reset(users)
}
