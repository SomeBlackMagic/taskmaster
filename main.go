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
)

var (
	cmd     *exec.Cmd
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
	command := flag.String("command", "", "Command to execute")
	flag.Parse()
	if command == nil {
		log.Fatalf("Please specify command via --command parameter")
	}
	go execute(*command)
	for {
		select {
		case code := <-bus:
			switch code {
			case -1:
				fmt.Printf("Interrupted. Code:%d", code)
			default:
				fmt.Printf("Shutting down. Code:%d", code)
			}
			close(bus)
			os.Exit(code)
			return
		case sig := <-signals:
			if cmd == nil {
				continue
			}
			log.Printf("Got signal, sending to inner app:%+v", sig)
			switch sig {
			case os.Kill, os.Interrupt:
				if err := cmd.Process.Signal(sig); err != nil {
					log.Fatalf("Error forwarding signal to internal app:%s", err)
				}
			case syscall.SIGTERM:
				if err := syscall.Kill(cmd.Process.Pid, syscall.SIGTERM); err != nil {
					log.Fatalf("Error forwarding signal to internal app:%s", err)
				}
			case syscall.SIGKILL:
				if err := syscall.Kill(cmd.Process.Pid, syscall.SIGKILL); err != nil {
					log.Fatalf("Error forwarding signal to internal app:%s", err)
				}
			}
		}
	}
}

func execute(rawCommand string) {
	command := unpackCommand(rawCommand)
	for {
		if len(command) > 1 {
			cmd = exec.Command(command[0], command[1:]...)
		} else if len(command) == 1 {
			cmd = exec.Command(command[0])
		} else {
			log.Fatalf("Error unpacking command, nothing to execute")
		}
		cmd.Stdin = os.Stdin
		cmd.Stderr = os.Stderr
		if code := statusCode(cmd); code != 0 {
			bus <- code
			break
		}
		if err := cmd.Wait(); err != nil {
			bus <- 0
			break
		}
	}
}

func statusCode(cmd *exec.Cmd) int {
	if _, err := cmd.Output(); err != nil {
		if w, ok := err.(*exec.ExitError); ok {
			return w.ExitCode()
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
