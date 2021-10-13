package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	codeNoCommand    = 32
	codeEmptyCommand = 33
)

type Option func(o *Application)

type Application struct {
	bus         chan int
	cmd         *exec.Cmd
	timeoutUnit time.Duration
	timeout     time.Duration
	command     []string
}

func main() {
	cmd := flag.String("command", "", "Command to execute")
	timeout := flag.Int("timeout", 0, "timeout of max execution")
	flag.Parse()
	bus := make(chan int)
	signalBus := make(chan os.Signal)
	app := NewApplication(
		bus,
		WithCommand(cmd),
		WithTimeout(timeout),
	)
	for _, s := range []os.Signal{os.Interrupt, syscall.SIGTERM, os.Kill} {
		signal.Notify(signalBus, s)
	}
	go app.start()
	for {
		select {
		case code := <-bus:
			fmt.Printf("Shutting down. Code:%d", code)
			close(bus)
			os.Exit(code)
			return
		case sig := <-signalBus:
			log.Printf("Got signal! %+v", sig)
			app.notify(sig)
		}
	}

}

func NewApplication(bus chan int, opts ...Option) Application {
	app := Application{
		bus:         bus,
		timeoutUnit: time.Second,
	}
	for _, opt := range opts {
		opt(&app)
	}
	return app
}

func (a *Application) notify(s os.Signal) {
	if a.cmd != nil {
		if err := a.cmd.Process.Signal(s); err != nil {
			log.Println("Error transfering signal" + err.Error())
		}
	}
}

func (a Application) start() {
	go a.setTimeout()
	if code := a.validate(); code != 0 {
		a.bus <- 12
	}
	a.execute()
}

func (a *Application) setTimeout() {
	if a.timeout > 0 {
		go func(t time.Duration) {
			time.Sleep(t)
			a.bus <- 12
		}(a.timeout)
	} else {
		log.Println("Has skipped timeout option")
	}
}

func (a Application) validate() int {
	if a.command == nil {
		return codeNoCommand
	}
	if len(a.command) == 0 {
		return codeEmptyCommand
	}
	return 0
}

func (a *Application) execute() {
	for {
		if len(a.command) > 1 {
			a.cmd = exec.Command(a.command[0], a.command[1:]...)
		} else {
			a.cmd = exec.Command(a.command[0])
		}
		if code := statusCode(a.cmd); code != 0 {
			a.bus <- code
			break
		}
		if err := a.cmd.Wait(); err != nil {
			log.Println("Done")
		}
	}
}

func WithTimeout(timeout *int) Option {
	return func(o *Application) {
		if timeout == nil {
			return
		}
		if *timeout == 0 {
			return
		}
		o.timeout = time.Duration(*timeout) * o.timeoutUnit
	}
}

func WithCommand(cmd *string) Option {
	return func(o *Application) {
		if cmd == nil {
			return
		}
		if *cmd == "" {
			return
		}
		o.command = unpackCommand(*cmd)
	}
}

func statusCode(cmd *exec.Cmd) int {
	if _, err := cmd.Output(); err != nil {
		if werr, ok := err.(*exec.ExitError); ok {
			return werr.ExitCode()
		}
	}
	return 0
}

func unpackCommand(command string) (parts []string) {
	if !strings.Contains(command, " ") {
		parts = []string{command}
	} else {
		if parts = strings.Split(command, " "); len(parts) == 0 {
			return nil
		}
	}
	return
}
