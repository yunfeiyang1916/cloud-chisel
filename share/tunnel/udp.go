package tunnel

import (
	"context"
	"encoding/gob"
	"io"
)

type udpPacket struct {
	Src     string
	Payload []byte
}

func init() {
	gob.Register(&udpPacket{})
}

// udp 通道，在流上对udp有效负载进行编码/解码
type udpChannel struct {
	r *gob.Decoder
	w *gob.Encoder
	c io.Closer
}

func (o *udpChannel) encode(src string, b []byte) error {
	return o.w.Encode(udpPacket{
		Src:     src,
		Payload: b,
	})
}

func (o *udpChannel) decode(p *udpPacket) error {
	return o.r.Decode(p)
}

// 判断上下文是否取消
func isDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
