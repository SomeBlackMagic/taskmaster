package runner_test

import (
	"testing"
	"time"

	"github.com/SomeBlackMagic/taskmaster/internal/runner"
)

func TestDefaultConfig(t *testing.T) {
	cfg := runner.DefaultConfig()

	if cfg.RestartDelay != time.Second {
		t.Errorf("RestartDelay: got %v, want %v", cfg.RestartDelay, time.Second)
	}
	if cfg.MaxRestarts != 0 {
		t.Errorf("MaxRestarts: got %d, want 0", cfg.MaxRestarts)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     runner.Config
		wantErr bool
	}{
		{
			name:    "valid single command",
			cfg:     runner.Config{Command: []string{"echo"}},
			wantErr: false,
		},
		{
			name:    "valid command with args",
			cfg:     runner.Config{Command: []string{"echo", "hello"}},
			wantErr: false,
		},
		{
			name:    "empty command",
			cfg:     runner.Config{},
			wantErr: true,
		},
		{
			name:    "negative restart delay",
			cfg:     runner.Config{Command: []string{"echo"}, RestartDelay: -1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
