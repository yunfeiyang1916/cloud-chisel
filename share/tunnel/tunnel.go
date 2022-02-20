package tunnel

import (
	"github.com/armon/go-socks5"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"golang.org/x/crypto/ssh"
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

// Tunnel 表示具有代理能力的SSH隧道, chisel的客户端和服务端都是隧道
// 客户端有一组远程映射，而凿子服务器有多组远程映射(每个客户端有一组)
// 每个remote都有一个到代理的1:1映射
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
