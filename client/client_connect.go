package chclient

import (
	"context"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/jpillora/backoff"
	chshare "github.com/yunfeiyang1916/cloud-chisel/share"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"golang.org/x/crypto/ssh"
	"io"
	"net"
	"strings"
	"time"
)

// 轮询连接
func (c *Client) connectionLoop(ctx context.Context) error {
	b := &backoff.Backoff{Max: c.config.MaxRetryInterval}
	for {
		connected, retry, err := c.connectionOnce(ctx)
		// 连接成功后复位backoff,也就是将attempt(尝试计数器)置为0
		if connected {
			b.Reset()
		}
		// connection error
		// 尝试计数器
		attempt := int(b.Attempt())
		// 最大尝试次数
		maxAttempt := c.config.MaxRetryCount
		// 不需要打印关闭连接的错误
		if strings.HasSuffix(err.Error(), "use of closed network connection") {
			err = io.EOF
		}
		// 显示错误消息和尝试计数(不包括断开连接)
		if err != nil && err != io.EOF {
			msg := fmt.Sprintf("Connection error: %s", err)
			if attempt > 0 {
				msg += fmt.Sprintf(" (Attempt: %d", attempt)
				if maxAttempt > 0 {
					msg += fmt.Sprintf("/%d", maxAttempt)
				}
				msg += ")"
			}
			c.Infof(msg)
		}
		// 已放弃重试
		if !retry || (maxAttempt >= 0 && attempt >= maxAttempt) {
			c.Infof("Give up")
			break
		}
		// 返回在尝试计数器递增之前的当前尝试的持续时间,尝试计数器已经递增
		d := b.Duration()
		c.Infof("Retrying in %s...", d)
		select {
		case <-cos.AfterSignal(d):
			continue //retry now
		case <-ctx.Done():
			c.Infof("Cancelled")
			return nil
		}
	}
	c.Close()
	return nil
}

// 连接 chisel server 并阻塞
func (c *Client) connectionOnce(ctx context.Context) (connected, retry bool, err error) {
	// already closed?
	select {
	case <-ctx.Done():
		return false, false, errors.New("Cancelled")
	default:
		// still open
	}
	ctx, cancle := context.WithCancel(ctx)
	defer cancle()
	// 准备拨号器
	d := websocket.Dialer{
		// 握手超时时间，默认45秒
		HandshakeTimeout: settings.EnvDuration("WS_TIMEOUT", 45*time.Second),
		Subprotocols:     []string{chshare.ProtocolVersion},
		TLSClientConfig:  c.tlsConfig,
		ReadBufferSize:   settings.EnvInt("WS_BUFF_SIZE", 0),
		WriteBufferSize:  settings.EnvInt("WS_BUFF_SIZE", 0),
	}
	// 设置代理
	if p := c.proxyURL; p != nil {
		if err := c.setProxy(p, &d); err != nil {
			return false, false, err
		}
	}
	wsConn, _, err := d.DialContext(ctx, c.server, c.config.Headers)
	if err != nil {
		return false, true, err
	}
	conn := cnet.NewWebSocketConn(wsConn)
	// 执行ssh握手
	c.Debugf("Handshaking...")
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, "", c.sshConfig)
	// 出错，判断是否可以重试
	if err != nil {
		e := err.Error()
		if strings.Contains(e, "unable to authenticate") {
			c.Infof("Authentication failed")
			c.Debugf(e)
			retry = false
		} else if strings.Contains(e, "connection abort") {
			c.Infof("retriable: %s", e)
			retry = true
		} else if n, ok := err.(net.Error); ok && !n.Temporary() {
			c.Infof(e)
			retry = false
		} else {
			c.Infof("retriable: %s", e)
			retry = true
		}
		return false, retry, err
	}
	defer sshConn.Close()
	// chisel client handshake (reverse of server handshake) send configuration
	c.Debugf("Sending config")
	t0 := time.Now()
	_, configerr, err := sshConn.SendRequest(
		"config",
		true,
		settings.EncodeConfig(c.computed),
	)
	if err != nil {
		c.Infof("Config verification failed")
		return false, false, err
	}
	if len(configerr) > 0 {
		return false, false, errors.New(string(configerr))
	}
	// 连接延迟时长
	c.Infof("Connected (Latency %s)", time.Since(t0))
	// 移交SSH连接以便隧道使用，阻断
	retry = true
	err = c.tunnel.BindSSH(ctx, sshConn, reqs, chans)
	if n, ok := err.(net.Error); ok && !n.Temporary() {
		retry = false
	}
	// 已分离连接
	c.Infof("Disconnected")
	connected = time.Since(t0) > 5*time.Second
	return connected, retry, err
}
