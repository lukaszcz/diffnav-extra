package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ThemeAuto  = "auto"
	ThemeLight = "light"
	ThemeDark  = "dark"
)

type UIConfig struct {
	HideHeader            bool   `yaml:"hideHeader"`
	HideFooter            bool   `yaml:"hideFooter"`
	ShowFileTree          bool   `yaml:"showFileTree"`
	FileTreeWidth         int    `yaml:"fileTreeWidth"`
	SearchTreeWidth       int    `yaml:"searchTreeWidth"`
	Icons                 string `yaml:"icons"`                 // "nerd-fonts-status" (default), "nerd-fonts-simple", "nerd-fonts-filetype", "nerd-fonts-full", "unicode", "ascii"
	ColorFileNames        bool   `yaml:"colorFileNames"`        // Color filenames by git status (default: true)
	ShowDiffStats         bool   `yaml:"showDiffStats"`         // Show the amount of lines added / removed next to the file
	SideBySide            bool   `yaml:"sideBySide"`            // Side-by-side diff view (default: true)
	Theme                 string `yaml:"theme"`                 // "auto" (default), "light", "dark"
	StartFoldersOpenDepth int    `yaml:"startFoldersOpenDepth"` // How many levels of folders to open on start (-1 = all, 0 = none)
}

type WatchConfig struct {
	Enabled  bool
	Cmd      string
	Interval time.Duration
}

type Config struct {
	UI    UIConfig    `yaml:"ui"`
	Watch WatchConfig `yaml:"-"`
}

func DefaultConfig() Config {
	return Config{
		UI: UIConfig{
			HideHeader:            false,
			HideFooter:            false,
			ShowFileTree:          true,
			FileTreeWidth:         30,
			SearchTreeWidth:       50,
			Icons:                 "nerd-fonts-status",
			ColorFileNames:        true,
			SideBySide:            true,
			ShowDiffStats:         true,
			Theme:                 ThemeAuto,
			StartFoldersOpenDepth: -1,
		},
	}
}

func NormalizeTheme(theme string) string {
	switch strings.ToLower(strings.TrimSpace(theme)) {
	case "", ThemeAuto:
		return ThemeAuto
	case ThemeLight:
		return ThemeLight
	case ThemeDark:
		return ThemeDark
	default:
		return ThemeAuto
	}
}

func ResolveTheme(configTheme string) string {
	if envTheme := os.Getenv("DIFFNAV_THEME"); envTheme != "" {
		return NormalizeTheme(envTheme)
	}
	return NormalizeTheme(configTheme)
}

func getConfigFilePath() string {
	var configDirs []string

	// Environment variable override - useful for development or non-standard setups.
	if dir := os.Getenv("DIFFNAV_CONFIG_DIR"); dir != "" {
		if s, err := os.Stat(dir); err == nil && s.IsDir() {
			return filepath.Join(dir, "config.yml")
		}
	}

	// Platform-specific config directories (macOS, Linux, etc.).
	configDirs = append(configDirs, platformConfigDirs()...)

	// Return the first config file that exists.
	for _, dir := range configDirs {
		configPath := filepath.Join(dir, "diffnav", "config.yml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	// If no config file exists, return the preferred path for creation.
	if len(configDirs) > 0 {
		return filepath.Join(configDirs[0], "diffnav", "config.yml")
	}
	return ""
}

func Load() Config {
	cfg := DefaultConfig()

	configPath := getConfigFilePath()
	if configPath == "" {
		return cfg
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return cfg
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig()
	}

	return cfg
}
