package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var items = []string{
	"Some",
	"Mystical",
}

func main() {
	bus := make(chan int, 1)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)

	go work(bus)

	select {
	case sig := <-signals:
		log.Printf("[testApp] got signal, shutting down: %v", sig)
		time.Sleep(5 * time.Second)
		log.Printf("[testApp] shutdown done")
		os.Exit(0)
	case code := <-bus:
		log.Printf("[testApp] normal exit: %d", code)
		os.Exit(code)
	}
}

func work(bus chan<- int) {
	for _, v := range items {
		log.Println("[testApp]", v)
		time.Sleep(1 * time.Second)
	}
	bus <- 0
}
