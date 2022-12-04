package tunnel

import (
	"context"
	"io"
	"net"

	"github.com/jpillora/sizestr"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

// ssh 隧道接口，Tunnel子类型
type sshTunnel interface {
	getSSH(ctx context.Context) ssh.Conn
	onConnectFunc(localPort string, logger *cio.Logger)
	onCloseFunc(localPort string, logger *cio.Logger)
}

type Proxy struct {
	*cio.Logger
	// ssh 隧道
	sshTun sshTunnel
	id     int
	count  int
	remote *settings.Remote
	dialer net.Dialer
	// tcp 侦听器
	tcp *net.TCPListener
	udp *udpListener
}

// NewProxy 创建代理并开始监听
func NewProxy(logger *cio.Logger, sshTun sshTunnel, index int, remote *settings.Remote) (*Proxy, error) {
	id := index + 1
	p := &Proxy{
		Logger: logger.Fork("proxy#%s", remote.String()),
		sshTun: sshTun,
		id:     id,
		remote: remote,
	}
	return p, p.listen()
}

func (p *Proxy) listen() error {
	if p.remote.Stdio {
		//TODO check if pipes active?
	} else if p.remote.LocalProto == "tcp" {
		addr, err := net.ResolveTCPAddr("tcp", p.remote.LocalHost+":"+p.remote.LocalPort)
		if err != nil {
			return p.Errorf("resolve: %s", err)
		}
		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return p.Errorf("tcp: %s", err)
		}
		p.Infof("Listening")
		p.tcp = l
	} else if p.remote.LocalProto == "udp" {
		l, err := listenUDP(p.Logger, p.sshTun, p.remote)
		if err != nil {
			return err
		}
		p.Infof("Listening")
		p.udp = l
	} else {
		return p.Errorf("unknown local proto")
	}
	return nil
}

// Run 启用代理并阻止其活动，通过取消上下文来关闭代理
func (p *Proxy) Run(ctx context.Context) error {
	if p.remote.Stdio {
		return p.runStdio(ctx)
	} else if p.remote.LocalProto == "tcp" {
		return p.runTCP(ctx)
	} else if p.remote.LocalProto == "udp" {
		return p.udp.run(ctx)
	}
	panic("should not get here")
}

func (p *Proxy) runStdio(ctx context.Context) error {
	defer p.Infof("Closed")
	for {
		p.pipeRemote(ctx, cio.Stdio)
		select {
		case <-ctx.Done():
			return nil
		default:
			// the connection is not ready yet, keep waiting
		}
	}
}

func (p *Proxy) runTCP(ctx context.Context) error {
	done := make(chan struct{})
	//implements missing net.ListenContext
	go func() {
		select {
		case <-ctx.Done():
			p.tcp.Close()
		case <-done:
		}
	}()
	for {
		src, err := p.tcp.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				//listener closed
				err = nil
			default:
				p.Infof("Accept error: %s", err)
			}
			close(done)
			return err
		}
		go p.pipeRemote(ctx, src)
	}
}

// 远程管道
func (p *Proxy) pipeRemote(ctx context.Context, src io.ReadWriteCloser) {
	defer src.Close()
	p.count++
	cid := p.count
	l := p.Fork("conn#%d", cid)
	l.Debugf("Open")
	p.sshTun.onConnectFunc(p.remote.LocalPort, l)
	sshConn := p.sshTun.getSSH(ctx)
	if sshConn == nil {
		l.Debugf("No remote connection")
		return
	}
	// 此代理的远程TCP连接的SSH请求
	dst, reqs, err := sshConn.OpenChannel("chisel", []byte(p.remote.Remote()))
	if err != nil {
		l.Infof("Stream error: %s", err)
		return
	}
	// 读取来自传入通道的所有请求，并响应false
	go ssh.DiscardRequests(reqs)
	//then pipe
	s, r := cio.Pipe(src, dst)
	l.Debugf("Close (sent %s received %s)", sizestr.ToString(s), sizestr.ToString(r))
	p.sshTun.onCloseFunc(p.remote.LocalPort, l)
}
