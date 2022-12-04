package main

import (
	"context"
	"log"
	"time"

	"github.com/yunfeiyang1916/cloud-chisel/share/cio"

	chclient "github.com/yunfeiyang1916/cloud-chisel/client"
)

func main() {
	c := &chclient.Config{
		Server:           "localhost:28888",
		Remotes:          []string{"R:0.0.0.0:28080:www.baidu.com:80"},
		Auth:             "9af92df4-e427-4086-9841-08da393c0f5c:b5fbcf537ed1a0d284fb6c1e236de0a4",
		KeepAlive:        25 * time.Second,
		MaxRetryInterval: time.Minute,
		MaxRetryCount:    -1,
		OnForwardingConnect: func(localPort string, logger *cio.Logger) {
			logger.Infof("%s 转发打开", localPort)
		},
		OnForwardingClose: func(localPort string, logger *cio.Logger) {
			logger.Infof("%s 转发关闭", localPort)
		},
	}
	//testProxy(c)
	//testPing(c)
	//testAll(c)
	testReverse(c)
	client, err := chclient.NewClient(c)
	if err != nil {
		log.Fatalln(err)
	}
	client.Debug = true
	//time.AfterFunc(10*time.Second, func() {
	//	os.Exit(0)
	//})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err = client.Start(ctx); err != nil {
		log.Fatalln("client.Start err:", err)
	}
	if err = client.Wait(); err != nil {
		log.Fatalln("client.Wait err:", err)
	}
}

func testAll(c *chclient.Config) {
	c.Remotes = []string{"127.0.0.1:40000:mycorp.dev.ztw.splashtop.com:80"}
	c.Auth = "all:all"
}

func testReverse(c *chclient.Config) {
	c.Remotes = []string{"R:0.0.0.0:7000:www.baidu.com:80"}
	c.Auth = "ping:pong"
}

func testPing(c *chclient.Config) {
	c.Remotes = []string{"127.0.0.1:40000:0.0.0.0:4000"}
	c.Auth = "ping:pong"
}

func testProxy(c *chclient.Config) {
	c.Remotes = []string{"80"}
	c.Auth = "9af92df4-e427-4086-9841-08da393c0f5c:b5fbcf537ed1a0d284fb6c1e236de0a4"
	c.Server = "118.24.6.169:80"
}
