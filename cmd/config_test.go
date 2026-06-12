package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateCommandReturnsErrorOnInvalidConfig(t *testing.T) {
	oldCfgFile := cfgFile
	cfgFile = filepath.Join(t.TempDir(), "config.json")
	t.Cleanup(func() { cfgFile = oldCfgFile })

	if err := os.WriteFile(cfgFile, []byte("{"), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	if err := validateCmd.RunE(validateCmd, nil); err == nil {
		t.Fatal("validate command should return an error for invalid config")
	}
}
