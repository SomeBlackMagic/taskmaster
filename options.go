package main

import "time"

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

func WithInterval(interval *int) Option {
	return func(o *Application) {
		if interval == nil {
			return
		}
		if *interval == 0 {
			return
		}
		o.interval = time.Duration(*interval)
	}
}
