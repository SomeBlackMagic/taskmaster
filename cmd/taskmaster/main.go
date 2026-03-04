package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/SomeBlackMagic/taskmaster/internal/runner"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: taskmaster <command> [args...]")
		os.Exit(1)
	}

	cfg := runner.DefaultConfig()
	cfg.Command = os.Args[1:]

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// signal.NotifyContext cancels ctx on SIGTERM or SIGINT, which propagates
	// to exec.CommandContext and stops the child process (rule #41).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	r := runner.New(cfg, logger)

	if err := r.Run(ctx); err != nil {
		logger.Error("runner stopped with error", "err", err)
		os.Exit(1)
	}
}
