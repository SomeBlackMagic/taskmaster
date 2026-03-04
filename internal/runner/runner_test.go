package runner_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/SomeBlackMagic/taskmaster/internal/runner"
)

// newDiscardLogger returns a logger that discards all output, suitable for tests.
func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRunnerStopsOnContextCancel verifies that Run returns nil when the
// context is cancelled while the child process is running.
func TestRunnerStopsOnContextCancel(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"sleep", "100"}

	r := runner.New(cfg, newDiscardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := r.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
}

// TestRunnerRestartsOnExitZero verifies that the runner restarts the process
// when it exits with code 0 and only stops on context cancellation.
func TestRunnerRestartsOnExitZero(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"true"}
	cfg.RestartDelay = 10 * time.Millisecond

	r := runner.New(cfg, newDiscardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := r.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil (restart loop should return nil on ctx cancel)", err)
	}
}

// TestRunnerStopsOnNonZeroExit verifies that Run returns an error when the
// child process exits with a non-zero exit code.
func TestRunnerStopsOnNonZeroExit(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"false"} // always exits with code 1

	r := runner.New(cfg, newDiscardLogger())

	err := r.Run(context.Background())
	if err == nil {
		t.Error("Run() error = nil, want non-nil for non-zero exit code")
	}
}

// TestRunnerInvalidCommand verifies that Run returns an error when the
// command binary does not exist.
func TestRunnerInvalidCommand(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"this-binary-does-not-exist"}

	r := runner.New(cfg, newDiscardLogger())

	err := r.Run(context.Background())
	if err == nil {
		t.Error("Run() error = nil, want non-nil for missing binary")
	}
}

// TestRunnerSignal verifies that Signal can be called safely before the child
// process starts (nil cmd) without panicking.
func TestRunnerSignalBeforeStart(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"sleep", "100"}

	r := runner.New(cfg, newDiscardLogger())

	// Signal before Run — must not panic or return an error.
	if err := r.Signal(nil); err != nil {
		t.Errorf("Signal() before start error = %v, want nil", err)
	}
}
