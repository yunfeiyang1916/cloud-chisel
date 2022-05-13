package main

import (
	"context"
	chclient "github.com/yunfeiyang1916/cloud-chisel/client"
	"log"
	"time"
)

func main() {
	c := chclient.Config{
		Server:           "localhost:28888",
		Remotes:          []string{"R:0.0.0.0:28080:www.baidu.com:80"},
		Auth:             "9af92df4-e427-4086-9841-08da393c0f5c:b5fbcf537ed1a0d284fb6c1e236de0a4",
		KeepAlive:        25 * time.Second,
		MaxRetryInterval: time.Minute,
		MaxRetryCount:    -1,
	}
	client, err := chclient.NewClient(&c)
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
