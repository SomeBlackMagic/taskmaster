package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
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

var (
	signals chan os.Signal
	listen  = []os.Signal{
		os.Interrupt,
		syscall.SIGTERM,
		os.Kill,
		syscall.SIGKILL,
	}
)

func init() {
	signals = make(chan os.Signal)
	for _, s := range listen {
		signal.Notify(signals, s)
	}
}

func main() {
	go work()
	for {
		select {
		case sig := <-signals:
			log.Printf("[testApp] Got signal. Shutdown app: %+v", sig)
			time.Sleep(5 * time.Second)
			log.Printf("[testApp] Shutdown done. exit: ")
			os.Exit(0)
			return
		}
	}
}

func work() {
	for _, v := range strings {
		log.Println("[testApp] ", v)
		time.Sleep(2 * time.Second)
	}
	log.Fatalf("[testApp] And prepares to attack")
}
