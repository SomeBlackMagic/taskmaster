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
	//"Monster",
	//"Goes",
	//"Around",
	//"The",
	//"Cave",
}

var (
	bus     chan int
	signals chan os.Signal
	listen  = []os.Signal{
		os.Interrupt,
		syscall.SIGTERM,
		os.Kill,
		syscall.SIGKILL,
	}
)

func init() {
	bus = make(chan int)
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
		case code := <-bus:
			log.Printf("[testApp] Normal shutdown:  %+v", code)
			os.Exit(0)
			return
		}
	}
}

func work() {
	for _, v := range strings {
		log.Println("[testApp] ", v)
		time.Sleep(1 * time.Second)
	}
	bus <- 0
	return
}
