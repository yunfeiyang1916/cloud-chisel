package main

import (
	"context"
	"log"

	chserver "github.com/yunfeiyang1916/cloud-chisel/server"
)

func main() {
	config := &chserver.Config{
		AuthFile: "config/users.json",
		//Reverse:  true,
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
