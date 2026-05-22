//go:build !darwin

package config

import "os"

// platformConfigDirs returns OS-specific config directory paths for non-Darwin platforms.
func platformConfigDirs() []string {
	var dirs []string
	if configDir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, configDir)
	}
	return dirs
}
