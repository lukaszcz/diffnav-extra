//go:build darwin

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// unsetEnv removes an environment variable for the duration of the test,
// restoring its original value (if any) on cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	orig, wasSet := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if wasSet {
			os.Setenv(key, orig)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestGetConfigFilePathDarwinEnvOverride(t *testing.T) {
	// DIFFNAV_CONFIG_DIR set to a valid directory returns that path directly.
	dir := t.TempDir()
	t.Setenv("DIFFNAV_CONFIG_DIR", dir)

	path := getConfigFilePath()
	want := filepath.Join(dir, "config.yml")
	if path != want {
		t.Errorf("expected %q, got %q", want, path)
	}
}

func TestGetConfigFilePathDarwinEnvNotDir(t *testing.T) {
	// DIFFNAV_CONFIG_DIR pointing to a non-directory falls through to darwin paths.
	dir := t.TempDir()
	file := filepath.Join(dir, "notadir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFNAV_CONFIG_DIR", file)

	path := getConfigFilePath()
	// Should not use the non-directory as config dir.
	if path == filepath.Join(file, "config.yml") {
		t.Errorf("should not use non-directory DIFFNAV_CONFIG_DIR, got %q", path)
	}
	// Should still produce a valid path under a darwin config directory.
	if path == "" {
		t.Error("expected non-empty config path")
	}
}

func TestGetConfigFilePathDarwinXDG(t *testing.T) {
	// XDG_CONFIG_HOME set on darwin adds it to configDirs first.
	xdgDir := t.TempDir()
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	// Set a different HOME so os.UserConfigDir still resolves.
	t.Setenv("HOME", t.TempDir())

	path := getConfigFilePath()
	// XDG is the first entry in configDirs (both from darwin block and UserConfigDir).
	want := filepath.Join(xdgDir, "diffnav", "config.yml")
	if path != want {
		t.Errorf("expected %q, got %q", want, path)
	}
}

func TestGetConfigFilePathDarwinHome(t *testing.T) {
	// HOME set on darwin (without XDG) adds $HOME/.config to configDirs.
	homeDir := t.TempDir()
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	unsetEnv(t, "XDG_CONFIG_HOME")
	t.Setenv("HOME", homeDir)

	path := getConfigFilePath()
	// First dir is $HOME/.config (from darwin block); UserConfigDir adds Library/Application Support.
	want := filepath.Join(homeDir, ".config", "diffnav", "config.yml")
	if path != want {
		t.Errorf("expected %q, got %q", want, path)
	}
}

func TestGetConfigFilePathDarwinUserConfigDirFallback(t *testing.T) {
	// Without XDG, os.UserConfigDir on darwin returns $HOME/Library/Application Support.
	// Verify that UserConfigDir contributes to configDirs.
	homeDir := t.TempDir()
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	unsetEnv(t, "XDG_CONFIG_HOME")
	t.Setenv("HOME", homeDir)

	path := getConfigFilePath()
	// Should be non-empty and under homeDir.
	if path == "" {
		t.Error("expected non-empty config path")
	}
	if !strings.HasPrefix(path, homeDir) {
		t.Errorf("expected path under HOME %q, got %q", homeDir, path)
	}
}

func TestGetConfigFilePathDarwinNoDirs(t *testing.T) {
	// No XDG_CONFIG_HOME, no HOME: darwin block adds nothing, os.UserConfigDir fails.
	// Empty configDirs should return "".
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	unsetEnv(t, "XDG_CONFIG_HOME")
	unsetEnv(t, "HOME")

	path := getConfigFilePath()
	if path != "" {
		t.Errorf("expected empty string, got %q", path)
	}
}

func TestGetConfigFilePathDarwinExistingFile(t *testing.T) {
	// Config file found in the first existing directory.
	xdgDir := t.TempDir()
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	t.Setenv("HOME", t.TempDir())

	// Create config file in XDG dir (first in configDirs).
	diffnavDir := filepath.Join(xdgDir, "diffnav")
	if err := os.MkdirAll(diffnavDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(diffnavDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("ui:\n  hideHeader: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := getConfigFilePath()
	if path != configPath {
		t.Errorf("expected %q, got %q", configPath, path)
	}
}

func TestGetConfigFilePathDarwinExistingFileInSecondDir(t *testing.T) {
	// Config file not in first dir but found in a later dir.
	homeDir := t.TempDir()
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	unsetEnv(t, "XDG_CONFIG_HOME")
	t.Setenv("HOME", homeDir)

	// Do NOT create config in $HOME/.config/diffnav/ (first dir).
	// Create config in $HOME/Library/Application Support/diffnav/ (second dir, from UserConfigDir).
	libDir := filepath.Join(homeDir, "Library", "Application Support", "diffnav")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(libDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("ui:\n  hideHeader: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := getConfigFilePath()
	if path != configPath {
		t.Errorf("expected %q, got %q", configPath, path)
	}
}

func TestGetConfigFilePathDarwinFallbackFirstDir(t *testing.T) {
	// No config file exists anywhere; fallback to the first directory in configDirs.
	xdgDir := t.TempDir()
	unsetEnv(t, "DIFFNAV_CONFIG_DIR")
	t.Setenv("XDG_CONFIG_HOME", xdgDir)
	t.Setenv("HOME", t.TempDir())

	path := getConfigFilePath()
	want := filepath.Join(xdgDir, "diffnav", "config.yml")
	if path != want {
		t.Errorf("expected %q, got %q", want, path)
	}
}
