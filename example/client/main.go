package main

import (
	"context"
	chclient "github.com/yunfeiyang1916/cloud-chisel/client"
	"log"
)

func main() {
	c := chclient.Config{
		Server:  "https://chisel-demo.herokuapp.com",
		Remotes: []string{"3000"},
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
