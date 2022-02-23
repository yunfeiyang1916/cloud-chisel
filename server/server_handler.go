package chserver

import (
	chshare "github.com/yunfeiyang1916/cloud-chisel/share"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"github.com/yunfeiyang1916/cloud-chisel/share/tunnel"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// handleClientHandler 是主要的HTTP websocket处理器
func (s *Server) handleClientHandler(w http.ResponseWriter, r *http.Request) {
	// 将有chisel前缀的请求升级成websocket
	upgrade := strings.ToLower(r.Header.Get("Upgrade"))
	// WebSocket协议版本号
	protocol := r.Header.Get("Sec-WebSocket-Protocol")
	if upgrade == "websocket" && strings.HasPrefix(protocol, "chisel-") {
		if protocol == chshare.ProtocolVersion {
			// 转成websocket连接处理
			s.handleWebsocket(w, r)
			return
		}
		// 协议版本号已不匹配，不在处理
		s.Infof("ignored client connection using protocol '%s', expected '%s'", protocol, chshare.ProtocolVersion)
	}
	// 仅提供代理请求
	if s.reverseProxy != nil {
		s.reverseProxy.ServeHTTP(w, r)
		return
	}
	// 未定义代理，判断是否是健康检测或者版本检测
	switch r.URL.String() {
	case "/health":
		w.Write([]byte("OK\n"))
		return
	case "/version":
		w.Write([]byte(chshare.BuildVersion))
		return
	}
	// 不匹配，返回404
	w.WriteHeader(404)
	w.Write([]byte("Not found"))
}

// handleWebsocket 转成websocket连接处理
func (s *Server) handleWebsocket(w http.ResponseWriter, req *http.Request) {
	// 递增连接会话数量
	id := atomic.AddInt32(&s.sessCount, 1)
	l := s.Fork("session#%d", id)
	// 将http连接转成websocket连接
	wsConn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		l.Debugf("Failed to upgrade (%s)", err)
		return
	}
	// 封装ws连接
	conn := cnet.NewWebSocketConn(wsConn)
	// 进行ssh握手
	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.sshConfig)
	if err != nil {
		s.Debugf("Failed to handshake (%s)", err)
		return
	}
	// 提取user
	var user *settings.User
	if s.users.Len() > 0 {
		sid := string(sshConn.SessionID())
		u, ok := s.sessions.Get(sid)
		if !ok {
			panic("bug in ssh auth handler")
		}
		user = u
		s.sessions.Del(sid)
	}
	// chisel server handshake (reverse of client handshake)
	// verify configuration
	l.Debugf("Verifying configuration")
	// 等待连接请求，超时后会自动关闭连接
	var r *ssh.Request
	select {
	case r = <-reqs:
	case <-time.After(settings.EnvDuration("CONFIG_TIMEOUT", 10*time.Second)):
		l.Debugf("Timeout waiting for configuration")
		sshConn.Close()
		return
	}
	failed := func(err error) {
		l.Debugf("Failed: %s", err)
		r.Reply(false, []byte(err.Error()))
	}
	// 第一个请求不是config请求
	if r.Type != "config" {
		failed(s.Errorf("expecting config request"))
		return
	}
	c, err := settings.DecodeConfig(r.Payload)
	if err != nil {
		failed(s.Errorf("invalid config"))
		return
	}
	// 如果客户端的版本与服务端的构建版本不一致，则打印日志
	if c.Version != chshare.BuildVersion {
		v := c.Version
		if v == "" {
			v = "<unknown>"
		}
		l.Infof("Client version (%s) differs from server version (%s)", v, chshare.BuildVersion)
	}
	// 验证远程配置
	for _, r := range c.Remotes {
		// 如果设置了user，则确保该user有权限访问
		if user != nil {
			addr := r.UserAddr()
			if !user.HasAccess(addr) {
				failed(s.Errorf("access to '%s' denied", addr))
				return
			}
		}
		// 确认服务端是否允许反向隧道
		if r.Reverse && !s.config.Reverse {
			l.Debugf("Denied reverse port forwarding request, please enable --reverse")
			failed(s.Errorf("Reverse port forwaring not enabled on server"))
			return
		}
		// 确认反向隧道是否可用
		if r.Reverse && !r.CanListen() {
			failed(s.Errorf("Server cannot listen on %s", r.String()))
			return
		}
	}
	// 回复config验证通过
	r.Reply(true, nil)
	// 给每个ssh连接创建隧道
	tunnel := tunnel.New(tunnel.Config{
		Logger:    l,
		Inbound:   s.config.Reverse,
		Outbound:  true, // 服务器总是接受出站
		Socks:     s.config.Socks5,
		KeepAlive: s.config.KeepAlive,
	})
	// bind
	eg, ctx := errgroup.WithContext(req.Context())
	eg.Go(func() error {
		// 移交SSH连接以供隧道使用，并阻塞
		return tunnel.BindSSH(ctx, sshConn, reqs, chans)
	})
	eg.Go(func() error {
		serverInbound := c.Remotes.Reversed(true)
		if len(serverInbound) == 0 {
			return nil
		}
		// 阻塞
		return tunnel.BindRemotes(ctx, serverInbound)
	})
	err = eg.Wait()
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		l.Debugf("Closed connection (%s)", err)
	} else {
		l.Debugf("Closed connection")
	}
}
