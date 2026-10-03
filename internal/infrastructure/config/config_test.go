package config

import (
	"os"
	"testing"
)

// TestLoadValidation verifies configuration validation against environment variables (defaults, custom, invalid values).
func TestLoadValidation(t *testing.T) {
	tests := []struct {
		name    string
		port    string
		timeout string
		wantErr bool
	}{
		{name: "defaults"},
		{name: "valid custom values", port: "9090", timeout: "2s"},
		{name: "invalid port", port: "70000", wantErr: true},
		{name: "invalid timeout", timeout: "nope", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOST", "")
			t.Setenv("PORT", tt.port)
			t.Setenv("REQUEST_TIMEOUT", tt.timeout)
			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if !tt.wantErr && cfg.Port < 1 {
				t.Fatalf("unexpected port: %d", cfg.Port)
			}
		})
	}
}

// TestLoadDoesNotDependOnProcessEnvironment verifies default configuration values when env vars are unset.
func TestLoadDoesNotDependOnProcessEnvironment(t *testing.T) {
	for _, key := range []string{"HOST", "PORT", "REQUEST_TIMEOUT"} {
		os.Unsetenv(key)
	}
	cfg, err := Load()
	if err != nil || cfg.Host != "127.0.0.1" || cfg.Port != 8080 {
		t.Fatalf("defaults = %+v, err = %v", cfg, err)
	}
}
