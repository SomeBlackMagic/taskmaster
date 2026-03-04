package runner_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/SomeBlackMagic/taskmaster/internal/runner"
)

// panicStarter is a test double that panics on Start to test recovery.
type panicStarter struct{}

func (p *panicStarter) Start(_ context.Context, _ []string) error {
	panic("injected panic for testing")
}

// newDiscardLogger returns a logger that discards all output, suitable for tests.
func newDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestRunnerPreCancelledContext verifies that Run returns nil immediately
// when called with an already-cancelled context (covers top-of-loop guard).
func TestRunnerPreCancelledContext(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"sleep", "100"}

	r := runner.New(cfg, newDiscardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run

	if err := r.Run(ctx); err != nil {
		t.Errorf("Run() error = %v, want nil for pre-cancelled context", err)
	}
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

// TestRunnerGracefulShutdown verifies that cancelling the context causes
// Run to return nil within a reasonable time, not an error.
func TestRunnerGracefulShutdown(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"sleep", "100"}

	r := runner.New(cfg, newDiscardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- r.Run(ctx) }()

	// Give the process time to start.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run() error = %v, want nil on graceful shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Run() did not return within 2s after context cancel")
	}
}

// TestRunnerConcurrentSignal verifies that Signal can be called concurrently
// with Run without triggering a data race (validated by -race flag).
func TestRunnerConcurrentSignal(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"sleep", "100"}

	r := runner.New(cfg, newDiscardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	runDone := make(chan struct{})
	go func() { defer close(runDone); r.Run(ctx) }() //nolint:errcheck

	// Give the process time to start.
	time.Sleep(50 * time.Millisecond)

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Signal(syscall.SIGCONT) // SIGCONT is safe: no-op for running process
		}()
	}
	wg.Wait()
	<-runDone
}

// TestRunnerBackoff verifies that the restart delay is actually honoured
// between process restarts on exit code 0.
func TestRunnerBackoff(t *testing.T) {
	const delay = 150 * time.Millisecond

	cfg := runner.DefaultConfig()
	cfg.Command = []string{"true"}
	cfg.RestartDelay = delay
	cfg.MaxRestarts = 1 // one restart → delay fires exactly once

	r := runner.New(cfg, newDiscardLogger())

	start := time.Now()
	_ = r.Run(context.Background())
	elapsed := time.Since(start)

	if elapsed < delay {
		t.Errorf("elapsed %v < restart delay %v: backoff not applied", elapsed, delay)
	}
}

// TestRunnerPanicRecovery verifies that a panic inside the process loop is
// recovered and returned as an error instead of crashing the program.
func TestRunnerPanicRecovery(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"echo"}

	r := runner.NewWithStarter(cfg, newDiscardLogger(), &panicStarter{})

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil after panic")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("error = %q, want it to contain 'panic'", err.Error())
	}
}

// TestRunnerMaxRestarts verifies that the runner stops and returns an error
// after reaching the configured MaxRestarts limit on exit code 0.
func TestRunnerMaxRestarts(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"true"} // always exits with code 0
	cfg.RestartDelay = 0
	cfg.MaxRestarts = 2 // allow 2 restarts (3 total starts)

	r := runner.New(cfg, newDiscardLogger())

	err := r.Run(context.Background())
	if err == nil {
		t.Error("Run() error = nil, want non-nil after max restarts reached")
	}
}

// TestRunnerMaxRestartsZeroMeansUnlimited verifies that MaxRestarts=0 means
// unlimited restarts (default behavior).
func TestRunnerMaxRestartsZeroMeansUnlimited(t *testing.T) {
	cfg := runner.DefaultConfig()
	cfg.Command = []string{"true"}
	cfg.RestartDelay = 5 * time.Millisecond
	cfg.MaxRestarts = 0 // unlimited

	r := runner.New(cfg, newDiscardLogger())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := r.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil (unlimited restarts, stopped by ctx)", err)
	}
}
