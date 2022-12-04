package tunnel

import (
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/jpillora/sizestr"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/cnet"
	"github.com/yunfeiyang1916/cloud-chisel/share/settings"
	"golang.org/x/crypto/ssh"
)

// 处理ssh在正常数据流之外发送的请求，接收ping,响应pong。主要用于保活
func (t *Tunnel) handleSSHRequests(reqs <-chan *ssh.Request) {
	for r := range reqs {
		switch r.Type {
		case "ping":
			r.Reply(true, []byte("pong"))
		default:
			t.Debugf("Unknown request: %s", r.Type)
		}
	}
}

// 处理ssh管道
// ssh.NewChannel 表示一个通道的传入请求。它必须通过调用Accept来接受使用，或者通过调用Reject来拒绝使用
func (t *Tunnel) handleSSHChannels(chans <-chan ssh.NewChannel) {
	for ch := range chans {
		go t.handleSSHChannel(ch)
	}
}

// 处理ssh管道
// ssh.NewChannel 表示一个通道的传入请求。它必须通过调用Accept来接受使用，或者通过调用Reject来拒绝使用
func (t *Tunnel) handleSSHChannel(ch ssh.NewChannel) {
	if !t.Config.Outbound {
		t.Debugf("Denied outbound connection")
		ch.Reject(ssh.Prohibited, "Denied outbound connection")
		return
	}
	remote := string(ch.ExtraData())
	// 提取协议
	hostPort, proto := settings.L4Proto(remote)
	udp := proto == "udp"
	socks := hostPort == "socks"
	if socks && t.socksServer == nil {
		t.Debugf("Denied socks request, please enable socks")
		ch.Reject(ssh.Prohibited, "SOCKS5 is not enabled")
		return
	}
	sshChan, reqs, err := ch.Accept()
	if err != nil {
		t.Debugf("Failed to accept stream: %s", err)
		return
	}
	stream := io.ReadWriteCloser(sshChan)
	// cnet.MeterRWC(t.Logger.Fork("sshchan"), sshChan)
	defer stream.Close()
	go ssh.DiscardRequests(reqs)
	l := t.Logger.Fork("conn#%d", t.connStats.New())
	// ready to handle
	// 递增连接计数器打开数量
	t.connStats.Open()
	l.Debugf("Open %s", t.connStats.String())
	forwardingPort := ""
	if len(t.Remotes) > 0 {
		forwardingPort = t.Remotes[0].LocalPort
	}
	t.onConnectFunc(forwardingPort, l)
	if socks {
		err = t.handleSocks(stream)
	} else if udp {
		err = t.handleUDP(l, stream, hostPort)
	} else {
		err = t.handleTCP(l, stream, hostPort)
	}
	t.connStats.Close()
	errmsg := ""
	if err != nil && !strings.HasSuffix(err.Error(), "EOF") {
		errmsg = fmt.Sprintf(" (error %s)", err)
	}
	l.Debugf("Close %s%s", t.connStats.String(), errmsg)
	t.onCloseFunc(forwardingPort, l)
}

func (t *Tunnel) handleSocks(src io.ReadWriteCloser) error {
	return t.socksServer.ServeConn(cnet.NewRWCConn(src))
}
func (t *Tunnel) handleTCP(l *cio.Logger, src io.ReadWriteCloser, hostPort string) error {
	dst, err := net.Dial("tcp", hostPort)
	if err != nil {
		return err
	}
	s, r := cio.Pipe(src, dst)
	l.Debugf("sent %s received %s", sizestr.ToString(s), sizestr.ToString(r))
	return nil
}
