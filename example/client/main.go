package main

import (
	"context"
	chclient "github.com/yunfeiyang1916/cloud-chisel/client"
	"log"
	"time"
)

func main() {
	c := chclient.Config{
		Server:           "localhost:8888",
		Remotes:          []string{"3000"},
		Auth:             "foo:bar",
		MaxRetryInterval: time.Minute,
		MaxRetryCount:    -1,
	}
	client, err := chclient.NewClient(&c)
	if err != nil {
		log.Fatalln(err)
	}
	if err = client.Start(context.Background()); err != nil {
		log.Fatalln("client.Start err:", err)
	}
	if err = client.Wait(); err != nil {
		log.Fatalln("client.Wait err:", err)
	}
}
