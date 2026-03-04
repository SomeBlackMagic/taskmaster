package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Runner manages the lifecycle of a child process: starts it, restarts it on
// exit code 0, and stops it when the context is cancelled or the process
// exits with a non-zero code.
type Runner struct {
	cfg     Config
	logger  *slog.Logger
	starter ProcessStarter // injected; default is execStarter

	mu  sync.Mutex
	cmd *exec.Cmd // guarded by mu; only set by execStarter
}

// New creates a Runner with the given config and logger.
// All dependencies are injected; no global state is used.
func New(cfg Config, logger *slog.Logger) *Runner {
	r := &Runner{cfg: cfg, logger: logger}
	r.starter = &execStarter{r: r}
	return r
}

// NewWithStarter creates a Runner with a custom ProcessStarter.
// Intended for testing; production code should use New.
func NewWithStarter(cfg Config, logger *slog.Logger, starter ProcessStarter) *Runner {
	return &Runner{cfg: cfg, logger: logger, starter: starter}
}

// execStarter is the default ProcessStarter backed by os/exec.
// It also maintains r.cmd so that Runner.Signal works correctly.
type execStarter struct {
	r *Runner
}

func (e *execStarter) Start(ctx context.Context, command []string) error {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	// WaitDelay must be set before Start(); gives the process time to flush
	// output after context cancellation before SIGKILL is sent.
	cmd.WaitDelay = 5 * time.Second

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	e.r.setCmd(cmd)
	defer e.r.setCmd(nil)

	return cmd.Wait()
}

// Run starts the process management loop and blocks until it finishes.
// It returns nil if stopped by context cancellation, or an error if the
// process exits with a non-zero code or fails to start.
// ctx must be the first argument (rule #14).
func (r *Runner) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return r.loop(gctx)
	})
	return g.Wait()
}

// Signal forwards an OS signal to the running child process.
// It is safe to call before the process has started or after it has exited.
func (r *Runner) Signal(sig os.Signal) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	return r.cmd.Process.Signal(sig)
}

// loop is the main restart loop. It runs the process, restarts on exit 0,
// and stops on non-zero exit, context cancellation, or MaxRestarts reached.
// Panics inside the loop are recovered and returned as errors (rule #49).
func (r *Runner) loop(ctx context.Context) (retErr error) {
	defer func() {
		if p := recover(); p != nil {
			r.logger.ErrorContext(ctx, "panic recovered in runner loop", "panic", p)
			retErr = fmt.Errorf("panic in runner loop: %v", p)
		}
	}()

	restarts := 0

	for {
		// Check context before each iteration (rule #24).
		if ctx.Err() != nil {
			return nil
		}

		r.logger.InfoContext(ctx, "starting process", "command", r.cfg.Command, "restarts", restarts)

		err := r.runOnce(ctx)
		if err != nil {
			// Context was cancelled — treat as graceful stop, not an error.
			if ctx.Err() != nil {
				return nil
			}

			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				r.logger.InfoContext(ctx, "process exited with non-zero code",
					"code", exitErr.ExitCode())
				return fmt.Errorf("process exited with code %d: %w", exitErr.ExitCode(), err)
			}

			return fmt.Errorf("process error: %w", err)
		}

		// Exit code 0: check restart limit before sleeping (rule #47).
		if r.cfg.MaxRestarts > 0 && restarts >= r.cfg.MaxRestarts {
			r.logger.InfoContext(ctx, "max restarts reached, stopping",
				"max_restarts", r.cfg.MaxRestarts)
			return fmt.Errorf("max restarts (%d) reached", r.cfg.MaxRestarts)
		}

		restarts++
		r.logger.InfoContext(ctx, "process exited with code 0, restarting",
			"delay", r.cfg.RestartDelay, "restarts", restarts)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(r.cfg.RestartDelay):
		}
	}
}

// runOnce runs one instance of the child process via the injected starter.
func (r *Runner) runOnce(ctx context.Context) error {
	return r.starter.Start(ctx, r.cfg.Command)
}

// setCmd safely updates the current child process reference (rule #34).
func (r *Runner) setCmd(cmd *exec.Cmd) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cmd = cmd
}
