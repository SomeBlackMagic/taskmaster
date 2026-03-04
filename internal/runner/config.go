package runner

import (
	"errors"
	"time"
)

// Config holds the configuration for Runner.
type Config struct {
	// Command is the executable and its arguments to run.
	Command []string

	// RestartDelay is the pause between restarts on exit code 0.
	// Defaults to 1 second.
	RestartDelay time.Duration

	// MaxRestarts limits the number of restarts on exit code 0.
	// Zero means unlimited.
	MaxRestarts int
}

// DefaultConfig returns a Config with sane defaults.
func DefaultConfig() Config {
	return Config{
		RestartDelay: time.Second,
		MaxRestarts:  0,
	}
}

// Validate checks that the Config is valid before use.
func (c Config) Validate() error {
	if len(c.Command) == 0 {
		return errors.New("command must not be empty")
	}
	if c.RestartDelay < 0 {
		return errors.New("restart delay must not be negative")
	}
	return nil
}
