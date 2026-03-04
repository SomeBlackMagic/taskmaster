package runner

import "context"

// ProcessStarter abstracts running a single child process invocation.
// Declared in the consuming package (rule #3), kept minimal (rule #18).
// The default implementation uses os/exec; a fake may be injected in tests.
type ProcessStarter interface {
	// Start runs command and blocks until it exits.
	// Returns nil on exit code 0, *exec.ExitError on non-zero exit,
	// or a wrapped error if the process could not be started.
	Start(ctx context.Context, command []string) error
}
