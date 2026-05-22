package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.UI.HideHeader {
		t.Error("expected HideHeader=false")
	}
	if cfg.UI.HideFooter {
		t.Error("expected HideFooter=false")
	}
	if !cfg.UI.ShowFileTree {
		t.Error("expected ShowFileTree=true")
	}
	if cfg.UI.FileTreeWidth != 30 {
		t.Errorf("expected FileTreeWidth=30, got %d", cfg.UI.FileTreeWidth)
	}
	if cfg.UI.SearchTreeWidth != 50 {
		t.Errorf("expected SearchTreeWidth=50, got %d", cfg.UI.SearchTreeWidth)
	}
	if cfg.UI.Icons != "nerd-fonts-status" {
		t.Errorf("expected Icons=nerd-fonts-status, got %q", cfg.UI.Icons)
	}
	if !cfg.UI.ColorFileNames {
		t.Error("expected ColorFileNames=true")
	}
	if !cfg.UI.SideBySide {
		t.Error("expected SideBySide=true")
	}
	if !cfg.UI.ShowDiffStats {
		t.Error("expected ShowDiffStats=true")
	}
	if cfg.UI.Theme != ThemeAuto {
		t.Errorf("expected Theme=%q, got %q", ThemeAuto, cfg.UI.Theme)
	}
	if cfg.UI.StartFoldersOpenDepth != -1 {
		t.Errorf("expected StartFoldersOpenDepth=-1, got %d", cfg.UI.StartFoldersOpenDepth)
	}
}

func TestNormalizeTheme(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", ThemeAuto},
		{"auto", ThemeAuto},
		{"Auto", ThemeAuto},
		{"AUTO", ThemeAuto},
		{"  auto  ", ThemeAuto},
		{"light", ThemeLight},
		{"Light", ThemeLight},
		{"LIGHT", ThemeLight},
		{"dark", ThemeDark},
		{"Dark", ThemeDark},
		{"DARK", ThemeDark},
		{"solarized", ThemeAuto},
		{"nord", ThemeAuto},
	}
	for _, tc := range cases {
		got := NormalizeTheme(tc.input)
		if got != tc.want {
			t.Errorf("NormalizeTheme(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestResolveThemeEnvOverride(t *testing.T) {
	// Environment variable overrides config.
	t.Setenv("DIFFNAV_THEME", "light")
	got := ResolveTheme("dark")
	if got != ThemeLight {
		t.Errorf("expected env override to produce %q, got %q", ThemeLight, got)
	}
}

func TestResolveThemeNoEnv(t *testing.T) {
	got := ResolveTheme("dark")
	if got != ThemeDark {
		t.Errorf("expected %q, got %q", ThemeDark, got)
	}
}

func TestResolveThemeEmptyEnv(t *testing.T) {
	t.Setenv("DIFFNAV_THEME", "")
	got := ResolveTheme("light")
	if got != ThemeLight {
		t.Errorf("expected %q when env is empty, got %q", ThemeLight, got)
	}
}

func TestLoadNoConfigFile(t *testing.T) {
	// Point to a non-existent directory; Load should return defaults.
	t.Setenv("DIFFNAV_CONFIG_DIR", "/tmp/nonexistent_diffnav_config_dir_"+t.Name())
	cfg := Load()
	def := DefaultConfig()
	if cfg.UI != def.UI {
		t.Errorf("expected default UI config when no file exists, got %+v", cfg.UI)
	}
}

func TestLoadInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yml"
	if err := os.WriteFile(path, []byte("{{invalid yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFNAV_CONFIG_DIR", dir)
	cfg := Load()
	def := DefaultConfig()
	if cfg.UI != def.UI {
		t.Errorf("expected default config on invalid YAML, got %+v", cfg.UI)
	}
}

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config.yml"
	content := `ui:
  hideHeader: true
  hideFooter: true
  showFileTree: false
  fileTreeWidth: 40
  icons: ascii
  sideBySide: false
  theme: dark
  startFoldersOpenDepth: 0
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFNAV_CONFIG_DIR", dir)
	cfg := Load()
	if !cfg.UI.HideHeader {
		t.Error("expected HideHeader=true")
	}
	if !cfg.UI.HideFooter {
		t.Error("expected HideFooter=true")
	}
	if cfg.UI.ShowFileTree {
		t.Error("expected ShowFileTree=false")
	}
	if cfg.UI.FileTreeWidth != 40 {
		t.Errorf("expected FileTreeWidth=40, got %d", cfg.UI.FileTreeWidth)
	}
	if cfg.UI.Icons != "ascii" {
		t.Errorf("expected Icons=ascii, got %q", cfg.UI.Icons)
	}
	if cfg.UI.SideBySide {
		t.Error("expected SideBySide=false")
	}
	if cfg.UI.Theme != ThemeDark {
		t.Errorf("expected Theme=dark, got %q", cfg.UI.Theme)
	}
	if cfg.UI.StartFoldersOpenDepth != 0 {
		t.Errorf("expected StartFoldersOpenDepth=0, got %d", cfg.UI.StartFoldersOpenDepth)
	}
}

func TestGetConfigFilePathEnvOverride(t *testing.T) {
	dir := t.TempDir()
	// Create the directory so stat succeeds.
	t.Setenv("DIFFNAV_CONFIG_DIR", dir)
	path := getConfigFilePath()
	if path != dir+"/config.yml" {
		t.Errorf("expected %s/config.yml, got %s", dir, path)
	}
}

func TestGetConfigFilePathEnvNotDir(t *testing.T) {
	// DIFFNAV_CONFIG_DIR points to a file, not a directory.
	dir := t.TempDir()
	file := dir + "/notadir"
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFNAV_CONFIG_DIR", file)
	// Should fall through to standard paths.
	path := getConfigFilePath()
	if path == "" {
		t.Error("expected non-empty config path when env override is not a dir")
	}
}

func TestGetConfigFilePathStandardPath(t *testing.T) {
	// Without DIFFNAV_CONFIG_DIR, the function should find a path via
	// UserConfigDir or the fallback.
	restoreEnv(t, "DIFFNAV_CONFIG_DIR")
	path := getConfigFilePath()
	if path != "" && !strings.HasSuffix(path, "diffnav/config.yml") {
		t.Errorf("expected path ending in diffnav/config.yml, got %q", path)
	}
}

func TestLoadStandardConfigDirWithExistingFile(t *testing.T) {
	// Create a config file at a standard config dir location by pointing
	// XDG_CONFIG_HOME to a writable temp dir.
	restoreEnv(t, "DIFFNAV_CONFIG_DIR")
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	diffnavDir := filepath.Join(tmpDir, "diffnav")
	if err := os.MkdirAll(diffnavDir, 0o755); err != nil {
		t.Fatalf("cannot create config dir: %v", err)
	}
	configPath := filepath.Join(diffnavDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("ui:\n  hideHeader: true\n"), 0o644); err != nil {
		t.Fatalf("cannot write config file: %v", err)
	}

	cfg := Load()
	if !cfg.UI.HideHeader {
		t.Error("expected HideHeader=true from standard config dir")
	}
}

func TestLoadEmptyConfigPath(t *testing.T) {
	// Force getConfigFilePath to return "" by unsetting all config dir
	// sources, causing Load to return defaults early at the
	// configPath == "" check.
	restoreEnv(t, "DIFFNAV_CONFIG_DIR")
	restoreEnv(t, "XDG_CONFIG_HOME")
	restoreEnv(t, "HOME")
	cfg := Load()
	def := DefaultConfig()
	if cfg.UI != def.UI {
		t.Errorf("expected default config on empty config path, got %+v", cfg.UI)
	}
}

func TestLoadUnreadableFile(t *testing.T) {
	// Create a directory at the config path — os.ReadFile on a directory
	// always returns an error.
	dir := t.TempDir()
	path := dir + "/config.yml"
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFNAV_CONFIG_DIR", dir)
	cfg := Load()
	def := DefaultConfig()
	if cfg.UI != def.UI {
		t.Errorf("expected default config on unreadable path, got %+v", cfg.UI)
	}
}

func TestLoadReadsExistingFile(t *testing.T) {
	// Create a config file directly in DIFFNAV_CONFIG_DIR.
	dir := t.TempDir()
	path := dir + "/config.yml"
	content := "ui:\n  hideHeader: true\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DIFFNAV_CONFIG_DIR", dir)
	cfg := Load()
	if !cfg.UI.HideHeader {
		t.Error("expected HideHeader=true from loaded config")
	}
}

// restoreEnv saves the current value of key and unsets it, restoring on cleanup.
func restoreEnv(t *testing.T, key string) {
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

func TestGetConfigFilePathNoConfigDirs(t *testing.T) {
	// Without any config dir sources, getConfigFilePath should return "".
	restoreEnv(t, "DIFFNAV_CONFIG_DIR")
	restoreEnv(t, "XDG_CONFIG_HOME")
	restoreEnv(t, "HOME")

	path := getConfigFilePath()
	if path != "" {
		t.Errorf("expected empty string when no config dirs available, got %q", path)
	}
}

func TestGetConfigFilePathFindsExistingFile(t *testing.T) {
	// The for-loop should find an existing config file in a standard config dir.
	restoreEnv(t, "DIFFNAV_CONFIG_DIR")
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	diffnavDir := filepath.Join(tmpDir, "diffnav")
	if err := os.MkdirAll(diffnavDir, 0o755); err != nil {
		t.Fatalf("cannot create config dir: %v", err)
	}
	configPath := filepath.Join(diffnavDir, "config.yml")
	if err := os.WriteFile(configPath, []byte("ui:\n  hideHeader: true\n"), 0o644); err != nil {
		t.Fatalf("cannot write config file: %v", err)
	}

	path := getConfigFilePath()
	if path != configPath {
		t.Errorf("expected %q, got %q", configPath, path)
	}
}

func TestThemeConstants(t *testing.T) {
	if ThemeAuto != "auto" {
		t.Errorf("expected ThemeAuto=auto, got %q", ThemeAuto)
	}
	if ThemeLight != "light" {
		t.Errorf("expected ThemeLight=light, got %q", ThemeLight)
	}
	if ThemeDark != "dark" {
		t.Errorf("expected ThemeDark=dark, got %q", ThemeDark)
	}
}
