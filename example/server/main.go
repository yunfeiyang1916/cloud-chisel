package main

import (
	"context"
	chserver "github.com/yunfeiyang1916/cloud-chisel/server"
	"log"
	"time"
)

func main() {
	config := &chserver.Config{
		AuthFile:  "config/users.json",
		Reverse:   true,
		KeepAlive: 25 * time.Second,
	}
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
