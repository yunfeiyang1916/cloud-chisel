package tunnel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/armon/go-socks5"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"io/ioutil"
	"log"
	"os"
	"sync"
	"time"
)

// Config 隧道配置
type Config struct {
	*cio.Logger
	// 流入，入站
	Inbound bool
	// 流出，出站
	Outbound  bool
	Socks     bool
	KeepAlive time.Duration
}

// Tunnel 表示具有代理能力的SSH隧道, chisel的客户端和服务端都是隧道。
// 客户端有一组远程映射，而凿子服务器有多组远程映射(每个客户端有一组)。
// 每个remote都有一个1:1代理映射
// 代理通过ssh监听、发送数据，ssh连接的另一端与端点通信并返回响应。
type Tunnel struct {
	Config
	// 活跃连接互斥锁
	activeConnMut sync.RWMutex
	// 同步组
	activatingConn waitGroup
	// 活跃的ssh连接
	activeConn ssh.Conn
	// proxies
	proxyCount int
	// 连接计数器
	connStats cnet.ConnCount
	// Socks5代理
	socksServer *socks5.Server
}

func New(c Config) *Tunnel {
	c.Logger = c.Logger.Fork("tun")
	t := &Tunnel{
		Config: c,
	}
	t.activatingConn.Add(1)
	// 安装socks服务器(不监听任何端口!)
	extra := ""
	if c.Socks {
		sl := log.New(ioutil.Discard, "", 0)
		if t.Logger.Debug {
			sl = log.New(os.Stdout, "[socks]", log.Ldate|log.Ltime)
		}
		t.socksServer, _ = socks5.New(&socks5.Config{Logger: sl})
		extra += " (SOCKS enabled)"
	}
	t.Debugf("Created%s", extra)
	return t
}

// BindSSH 提供一个活动的SSH用于隧道使用
func (t *Tunnel) BindSSH(ctx context.Context, c ssh.Conn, reqs <-chan *ssh.Request, chans <-chan ssh.NewChannel) error {
	// link ctx to ssh-conn
	go func() {
		<-ctx.Done()
		if c.Close() == nil {
			t.Debugf("SSH cancelled")
		}
		t.activatingConn.DoneAll()
	}()
	t.activeConnMut.Lock()
	if t.activeConn != nil {
		panic("double bind ssh")
	}
	t.activeConn = c
	t.activeConnMut.Unlock()
	t.activatingConn.Done()
	if t.Config.KeepAlive > 0 {
		go t.keepAliveLoop(c)
	}
	// 处理ssh在正常数据流之外发送的请求，接收ping,响应pong。
	go t.handleSSHRequests(reqs)
	// 主要逻辑
	go t.handleSSHChannels(chans)
	t.Debugf("SSH connected")
	// 阻塞直到连接关闭
	err := c.Wait()
	t.Debugf("SSH disconnected")
	// mark inactive and block
	t.activatingConn.Add(1)
	t.activeConnMut.Lock()
	t.activeConn = nil
	t.activeConnMut.Unlock()
	return err
}

// 获取ssh连接，阻塞直到连接上
func (t *Tunnel) getSSH(ctx context.Context) ssh.Conn {
	// 是否已取消
	if isDone(ctx) {
		return nil
	}
	t.activeConnMut.RLock()
	c := t.activeConn
	t.activeConnMut.RUnlock()
	// 已经有连接了，直接返回
	if c != nil {
		return c
	}
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(settings.EnvDuration("SSH_WAIT", 35*time.Second)):
		return nil // 比SSH超时时间稍长
	case <-t.activatingConnWait():
		t.activeConnMut.RLock()
		c := t.activeConn
		t.activeConnMut.RUnlock()
		return c
	}
}

func (t *Tunnel) activatingConnWait() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		t.activatingConn.Wait()
		close(ch)
	}()
	return ch
}

// BindRemotes 将给定的远程服务转换为代理并阻塞，直到调用者通过取消上下文来关闭代理或出现代理错误后关闭
func (t *Tunnel) BindRemotes(ctx context.Context, remotes []*settings.Remote) error {
	if len(remotes) == 0 {
		return errors.New("no remotes")
	}
	if !t.Inbound {
		return errors.New("inbound connections blocked")
	}
	proxies := make([]*Proxy, len(remotes))
	for i, remote := range remotes {
		p, err := NewProxy(t.Logger, t, t.proxyCount, remote)
		if err != nil {
			return err
		}
		proxies[i] = p
		t.proxyCount++
	}
	// TODO: handle tunnel close
	eg, ctx := errgroup.WithContext(ctx)
	for _, proxy := range proxies {
		p := proxy
		eg.Go(func() error {
			return p.Run(ctx)
		})
	}
	t.Debugf("Bound proxies")
	err := eg.Wait()
	t.Debugf("Unbound proxies")
	return err
}

// 持续保活
func (t *Tunnel) keepAliveLoop(sshConn ssh.Conn) {
	msg := fmt.Sprintf("[LocalAddr:%s]=>[RemoteAddr:%s]", sshConn.LocalAddr(), sshConn.RemoteAddr())
	defer func() {
		// 在ping异常时关闭连接
		t.Errorf("%s,close ssh connection on abnormal ping", msg)
		sshConn.Close()
	}()
	// 一直ping，持续保活
	for {
		time.Sleep(t.Config.KeepAlive)
		// t.Infof("%s start send request keep alive", msg)
		select {
		case <-time.After(t.Config.KeepAlive):
			return
		case err := <-t.KeepAliveChan(sshConn):
			if err != nil {
				return
			}
		}
	}
}

func (t *Tunnel) KeepAliveChan(sshConn ssh.Conn) <-chan error {
	msg := fmt.Sprintf("[LocalAddr:%s]=>[RemoteAddr:%s]", sshConn.LocalAddr(), sshConn.RemoteAddr())
	ch := make(chan error)
	go func() {
		defer close(ch)
		_, b, err := sshConn.SendRequest("ping", true, nil)
		// t.Infof("%s end send request keep alive", msg)
		if err != nil {
			t.Errorf("%s ping error,err=%s", msg, err)
			ch <- err
		}
		// t.Infof("%s keep alive content %s", msg, string(b))
		if len(b) > 0 && !bytes.Equal(b, []byte("pong")) {
			t.Debugf("strange ping response")
			t.Errorf("%s strange ping response", msg)
			ch <- fmt.Errorf("strange ping response")
		}
	}()
	return ch
}

func (t *Tunnel) Close(ctx context.Context) error {
	sshConn := t.getSSH(ctx)
	if sshConn == nil {
		t.Debugf("No ssh-conn to close")
		return nil
	}
	return sshConn.Close()
}
