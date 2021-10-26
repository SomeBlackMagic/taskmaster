package main

import (
	"log"
	"time"
)

var strings = []string{
	"Some",
	"Mystical",
	"Monster",
	"Goes",
	"Around",
	"The",
	"Cave",
}

func main() {
	for _, v := range strings {
		log.Println(v)
		time.Sleep(10 * time.Second)
	}
	log.Fatalf("And prepares to attack")
}
