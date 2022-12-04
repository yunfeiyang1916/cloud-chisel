package main

import (
	"context"
	"github.com/yunfeiyang1916/cloud-chisel/share/cio"
	"github.com/yunfeiyang1916/cloud-chisel/share/tunnel"
	"log"

	chserver "github.com/yunfeiyang1916/cloud-chisel/server"
)

func main() {
	config := &chserver.Config{
		AuthFile: "config/users.json",
		Reverse:  true,
		OnConnect: func(localPort, remotePort string, tun *tunnel.Tunnel) {
			log.Println(localPort, "隧道建立")
		},
		OnClose: func(localPort string) {
			log.Println(localPort, "隧道关闭")
		},
		OnForwardingConnect: func(localPort string, logger *cio.Logger) {
			logger.Infof("%s 转发打开", localPort)
		},
		OnForwardingClose: func(localPort string, logger *cio.Logger) {
			logger.Infof("%s 转发关闭", localPort)
		},
	}
	//useProxy(config)
	s, err := chserver.NewServer(config)
	if err != nil {
		log.Fatal(err)
	}
	host := "0.0.0.0"
	port := "28888"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Debug = true
	if err := s.StartContext(ctx, host, port); err != nil {
		log.Fatal(err)
	}
	if err := s.Wait(); err != nil {
		log.Fatal(err)
	}
}

func useProxy(c *chserver.Config) {
	c.Proxy = "http://baidu.com"
	c.AuthFile = ""
}
