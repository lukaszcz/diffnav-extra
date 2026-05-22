//go:build darwin

package config

import (
	"os"
	"path/filepath"
)

// platformConfigDirs returns macOS-specific config directory paths.
// On macOS, check XDG_CONFIG_HOME first (if user explicitly set it),
// then fall back to ~/.config (common for CLI tools).
// os.UserConfigDir() already handles this for Linux.
func platformConfigDirs() []string {
	var dirs []string
	if xdgConfigDir := os.Getenv("XDG_CONFIG_HOME"); xdgConfigDir != "" {
		dirs = append(dirs, xdgConfigDir)
	}
	if home := os.Getenv("HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, ".config"))
	}
	if configDir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, configDir)
	}
	return dirs
}
