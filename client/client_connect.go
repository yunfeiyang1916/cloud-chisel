package chclient

import (
	"context"
	"github.com/jpillora/backoff"
)

// 轮询连接
func (c *Client) connectionLoop(ctx context.Context) error {
	b := &backoff.Backoff{Max: c.config.MaxRetryInterval}
	for {

	}
}
