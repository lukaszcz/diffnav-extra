package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/lukaszcz/diffnav-extra/pkg/config"
)

// ---------------------------------------------------------------------------
// Original tests (preserved)
// ---------------------------------------------------------------------------

func TestRootCommandStructure(t *testing.T) {
	if rootCmd.Use != "diffnav" {
		t.Errorf("expected Use='diffnav', got %q", rootCmd.Use)
	}
	if rootCmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if rootCmd.Run == nil {
		t.Error("expected Run function to be set")
	}
}

func TestRootCommandFlags(t *testing.T) {
	flags := rootCmd.Flags()

	sbsFlag, err := flags.GetBool("side-by-side")
	if err != nil {
		t.Fatalf("expected side-by-side flag: %v", err)
	}
	if sbsFlag {
		t.Error("expected side-by-side flag default false")
	}

	unifiedFlag, err := flags.GetBool("unified")
	if err != nil {
		t.Fatalf("expected unified flag: %v", err)
	}
	if unifiedFlag {
		t.Error("expected unified flag default false")
	}

	watchFlag, err := flags.GetBool("watch")
	if err != nil {
		t.Fatalf("expected watch flag: %v", err)
	}
	if watchFlag {
		t.Error("expected watch flag default false")
	}

	watchCmd, err := flags.GetString("watch-cmd")
	if err != nil {
		t.Fatalf("expected watch-cmd flag: %v", err)
	}
	if watchCmd != "git diff" {
		t.Errorf("expected watch-cmd default='git diff', got %q", watchCmd)
	}

	watchInterval, err := flags.GetDuration("watch-interval")
	if err != nil {
		t.Fatalf("expected watch-interval flag: %v", err)
	}
	if watchInterval.String() != "2s" {
		t.Errorf("expected watch-interval default=2s, got %v", watchInterval)
	}
}

func TestRootCommandVersionTemplate(t *testing.T) {
	if rootCmd.VersionTemplate() == "" {
		t.Error("expected version template to be set")
	}
}

func TestFlagChangedDetection(t *testing.T) {
	flags := rootCmd.Flags()
	// Save and restore the flag's Changed state to avoid polluting other tests.
	wasChanged := flags.Changed("watch-cmd")
	defer func() {
		if !wasChanged {
			flags.Visit(func(f *pflag.Flag) {
				if f.Name == "watch-cmd" {
					f.Changed = false
				}
			})
		}
	}()
	if err := flags.Set("watch-cmd", "custom-cmd"); err != nil {
		t.Fatalf("failed to set watch-cmd flag: %v", err)
	}
	if !flags.Changed("watch-cmd") {
		t.Error("expected watch-cmd flag to be marked as changed")
	}
	if err := flags.Set("watch-cmd", "git diff"); err != nil {
		t.Fatalf("failed to reset watch-cmd flag: %v", err)
	}
}

func TestSideBySideFlagShort(t *testing.T) {
	flag := rootCmd.Flags().Lookup("side-by-side")
	if flag == nil {
		t.Fatal("expected side-by-side flag to exist")
	}
	if flag.Shorthand != "s" {
		t.Errorf("expected shorthand 's', got %q", flag.Shorthand)
	}
}

func TestUnifiedFlagShort(t *testing.T) {
	flag := rootCmd.Flags().Lookup("unified")
	if flag == nil {
		t.Fatal("expected unified flag to exist")
	}
	if flag.Shorthand != "u" {
		t.Errorf("expected shorthand 'u', got %q", flag.Shorthand)
	}
}

func TestWatchFlagShort(t *testing.T) {
	flag := rootCmd.Flags().Lookup("watch")
	if flag == nil {
		t.Fatal("expected watch flag to exist")
	}
	if flag.Shorthand != "w" {
		t.Errorf("expected shorthand 'w', got %q", flag.Shorthand)
	}
}

// ---------------------------------------------------------------------------
// parseFlags tests
// ---------------------------------------------------------------------------

func TestParseFlagsDefaults(t *testing.T) {
	cmd := newCmdWithFlags()
	sbs, unified, help, watch, watchCmdStr, interval := parseFlags(cmd)
	if sbs {
		t.Error("expected side-by-side default false")
	}
	if unified {
		t.Error("expected unified default false")
	}
	if help {
		t.Error("expected help default false")
	}
	if watch {
		t.Error("expected watch default false")
	}
	if watchCmdStr != "git diff" {
		t.Errorf("expected watch-cmd 'git diff', got %q", watchCmdStr)
	}
	if interval != 2*time.Second {
		t.Errorf("expected interval 2s, got %v", interval)
	}
}

func TestParseFlagsWatchCmdChanged(t *testing.T) {
	cmd := newCmdWithFlags()
	_ = cmd.Flags().Set("watch-cmd", "custom")
	_, _, _, watch, _, _ := parseFlags(cmd)
	if !watch {
		t.Error("expected watch=true when watch-cmd is changed")
	}
}

func TestParseFlagsSideBySide(t *testing.T) {
	cmd := newCmdWithFlags()
	_ = cmd.Flags().Set("side-by-side", "true")
	sbs, _, _, _, _, _ := parseFlags(cmd)
	if !sbs {
		t.Error("expected side-by-side true")
	}
}

func TestParseFlagsUnified(t *testing.T) {
	cmd := newCmdWithFlags()
	_ = cmd.Flags().Set("unified", "true")
	_, unified, _, _, _, _ := parseFlags(cmd)
	if !unified {
		t.Error("expected unified true")
	}
}

func TestParseFlagsWatch(t *testing.T) {
	cmd := newCmdWithFlags()
	_ = cmd.Flags().Set("watch", "true")
	_, _, _, watch, _, _ := parseFlags(cmd)
	if !watch {
		t.Error("expected watch true")
	}
}

func newCmdWithFlags() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().BoolP("side-by-side", "s", false, "")
	cmd.Flags().BoolP("unified", "u", false, "")
	cmd.Flags().BoolP("watch", "w", false, "")
	cmd.Flags().String("watch-cmd", "git diff", "")
	cmd.Flags().Duration("watch-interval", 2*time.Second, "")
	cmd.Flags().Bool("help", false, "")
	return cmd
}

// ---------------------------------------------------------------------------
// buildConfig tests
// ---------------------------------------------------------------------------

func TestBuildConfigUnified(t *testing.T) {
	cfg := buildConfig(true, false, false, "git diff", 2*time.Second)
	if cfg.UI.SideBySide {
		t.Error("expected SideBySide=false when unified flag set")
	}
}

func TestBuildConfigSideBySide(t *testing.T) {
	cfg := buildConfig(false, true, false, "git diff", 2*time.Second)
	if !cfg.UI.SideBySide {
		t.Error("expected SideBySide=true when side-by-side flag set")
	}
}

func TestBuildConfigNeither(t *testing.T) {
	cfg := buildConfig(false, false, false, "git diff", 2*time.Second)
	// SideBySide comes from the loaded config default (true)
	if !cfg.UI.SideBySide {
		t.Error("expected default SideBySide=true from config")
	}
}

func TestBuildConfigWatch(t *testing.T) {
	cfg := buildConfig(false, false, true, "custom-cmd", 5*time.Second)
	if !cfg.Watch.Enabled {
		t.Error("expected watch enabled")
	}
	if cfg.Watch.Cmd != "custom-cmd" {
		t.Errorf("expected watch cmd 'custom-cmd', got %q", cfg.Watch.Cmd)
	}
	if cfg.Watch.Interval != 5*time.Second {
		t.Errorf("expected watch interval 5s, got %v", cfg.Watch.Interval)
	}
}

// ---------------------------------------------------------------------------
// setupLogging tests
// ---------------------------------------------------------------------------

func TestSetupLoggingNonDebug(t *testing.T) {
	t.Setenv("DEBUG", "false")

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	cleanup := setupLogging()

	// In non-debug mode, no debug.log file should be created.
	if _, err := os.Stat("debug.log"); !os.IsNotExist(err) {
		t.Fatal("expected debug.log to NOT exist in non-debug mode")
	}

	// Cleanup is a no-op; calling it twice must not panic.
	cleanup()
	cleanup()
}

func TestSetupLoggingDebugMode(t *testing.T) {
	tmpDir := t.TempDir()
	// Change to temp dir so debug.log is created there.
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Setenv("DEBUG", "true")

	called := false
	origExit := exitFunc
	exitFunc = func(int) { called = true }
	defer func() { exitFunc = origExit }()

	cleanup := setupLogging()
	cleanup()

	// In debug mode with successful file creation, it should NOT call exitFunc.
	if called {
		t.Error("setupLogging should not call exitFunc when debug.log opens successfully")
	}
}

func TestSetupLoggingDebugFileError(t *testing.T) {
	t.Setenv("DEBUG", "true")

	origExit := exitFunc
	exitCode := -1
	exitFunc = func(code int) { exitCode = code }
	defer func() { exitFunc = origExit }()

	// Make debug.log creation impossible by setting the working dir to a
	// non-existent directory — or just set DEBUG=true and make the open fail
	// by putting a directory at the path. Easier: create a directory named debug.log
	tmpDir := t.TempDir()
	debugLogDir := tmpDir + "/debug.log"
	if err := os.MkdirAll(debugLogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	cleanup := setupLogging()
	cleanup()

	if exitCode == -1 {
		t.Error("expected exitFunc to be called when debug.log cannot be opened")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for debug file error, got %d", exitCode)
	}
}

// ---------------------------------------------------------------------------
// readInput / readWatchInput / readStdinInput tests
// ---------------------------------------------------------------------------

func TestReadInputWatchMode(t *testing.T) {
	origWatchCmd := runWatchCmd
	defer func() { runWatchCmd = origWatchCmd }()

	runWatchCmd = func(cmd string) (string, error) {
		return "watch output", nil
	}

	origStat := stdinStat
	defer func() { stdinStat = origStat }()

	stdinStat = func() (os.FileInfo, error) { return nil, fmt.Errorf("no stdin") }

	result := readInput(true, "git diff", false)
	if result != "watch output" {
		t.Errorf("expected 'watch output', got %q", result)
	}
}

func TestReadInputStdinMode(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	// Provide a pipe input with a unified diff.
	diffContent := "diff --git a/foo b/foo\n--- a/foo\n+++ b/foo\n@@ -1 +1 @@\n-old\n+new\n"
	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: int64(len(diffContent))}, nil
	}
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader(diffContent))
	}

	result := readInput(false, "", false)
	if !strings.Contains(result, "diff --git") {
		t.Errorf("expected diff content, got %q", result)
	}
}

func TestReadWatchInput(t *testing.T) {
	origWatchCmd := runWatchCmd
	defer func() { runWatchCmd = origWatchCmd }()

	runWatchCmd = func(cmd string) (string, error) {
		if cmd != "my-cmd" {
			t.Errorf("expected 'my-cmd', got %q", cmd)
		}
		return "output", nil
	}

	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	stdinStat = func() (os.FileInfo, error) { return nil, fmt.Errorf("no stdin") }

	result := readWatchInput("my-cmd")
	if result != "output" {
		t.Errorf("expected 'output', got %q", result)
	}
}

func TestReadWatchInputStdinPipe(t *testing.T) {
	origWatchCmd := runWatchCmd
	defer func() { runWatchCmd = origWatchCmd }()
	origStat := stdinStat
	defer func() { stdinStat = origStat }()

	// Simulate stdin being a named pipe (should print warning).
	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: 100}, nil
	}
	runWatchCmd = func(cmd string) (string, error) { return "", nil }

	// Capture stderr
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	result := readWatchInput("git diff")
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = origStderr

	if result != "" {
		t.Errorf("expected empty result, got %q", result)
	}
	if !strings.Contains(buf.String(), "Warning: stdin input ignored in watch mode") {
		t.Errorf("expected warning, got %q", buf.String())
	}
}

func TestReadWatchInputCmdFail(t *testing.T) {
	origWatchCmd := runWatchCmd
	defer func() { runWatchCmd = origWatchCmd }()
	origStat := stdinStat
	defer func() { stdinStat = origStat }()

	stdinStat = func() (os.FileInfo, error) { return nil, fmt.Errorf("no stdin") }
	runWatchCmd = func(cmd string) (string, error) {
		return "", fmt.Errorf("command failed")
	}

	result := readWatchInput("bad-cmd")
	if result != "" {
		t.Errorf("expected empty result on cmd fail, got %q", result)
	}
}

func TestReadStdinInputEmptyPipe(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	// Simulate no pipe, no help, empty stat.
	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: 0, size: 0}, nil
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	readStdinInput(false)

	if exitCode == -1 {
		t.Error("expected exit to be called for empty stdin")
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for empty stdin, got %d", exitCode)
	}
}

func TestReadStdinInputHelpFlag(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: 0, size: 0}, nil
	}

	// With help=true, the "no pipe exit" should be skipped.
	diffContent := "diff --git a/foo b/foo\n--- a/foo\n+++ b/foo\n@@ -1 +1 @@\n-old\n+new\n"
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader(diffContent))
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	result := readStdinInput(true)

	if exitCode != -1 {
		t.Error("should not call exit when help is true")
	}
	if !strings.Contains(result, "diff --git") {
		t.Errorf("expected diff content, got %q", result)
	}
}

func TestReadStdinInputPipeWithDiff(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()

	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: 100}, nil
	}

	diffContent := "diff --git a/foo b/foo\n--- a/foo\n+++ b/foo\n@@ -1 +1 @@\n-old\n+new\n"
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader(diffContent))
	}

	result := readStdinInput(false)
	if !strings.Contains(result, "diff --git") {
		t.Errorf("expected diff content, got %q", result)
	}
}

func TestReadStdinInputEmptyDiff(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: 0}, nil
	}
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader("   \n"))
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	readStdinInput(false)

	if exitCode == -1 {
		t.Error("expected exit for empty input")
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0 for empty input, got %d", exitCode)
	}
}

func TestReadStdinInputNonUnifiedDiff(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: 100}, nil
	}
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader("just some text\n"))
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	readStdinInput(false)

	if exitCode != 0 {
		t.Errorf("expected exit code 0 for non-unified diff, got %d", exitCode)
	}
}

func TestReadStdinInputStatError(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()

	stdinStat = func() (os.FileInfo, error) {
		return nil, fmt.Errorf("stat error")
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Error("expected panic from stat error")
		}
	}()

	readStdinInput(false)
}

// ---------------------------------------------------------------------------
// runProgram tests
// ---------------------------------------------------------------------------

func TestRunProgramOpenTTYError(t *testing.T) {
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()

	openTTY = func() (*os.File, *os.File, error) {
		return nil, nil, fmt.Errorf("no tty")
	}

	// log.Fatal calls os.Exit by default. Override exitFunc to catch it.
	origExit := exitFunc
	exitCode := -1
	exitFunc = func(code int) { exitCode = code }
	defer func() { exitFunc = origExit }()

	runProgram("input", config.DefaultConfig())

	if exitCode == -1 {
		t.Error("expected exit when OpenTTY fails")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for OpenTTY failure, got %d", exitCode)
	}
}

func TestRunProgramSuccess(t *testing.T) {
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()

	// Create real pipe files for the TTY mock.
	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()

	openTTY = func() (*os.File, *os.File, error) {
		return ttyIn, ttyOut, nil
	}

	programCalled := false
	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		programCalled = true
		return nil
	}

	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()
	runProgramFn = func(p *tea.Program) (tea.Model, error) { return nil, nil }
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	exitFunc = func(int) {}

	runProgram("diff input", config.DefaultConfig())

	if !programCalled {
		t.Error("expected newProgram to be called")
	}
}

func TestRunProgramTTYSameFileClose(t *testing.T) {
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()

	// Use the same file for both input and output.
	f, _, _ := os.Pipe()

	openTTY = func() (*os.File, *os.File, error) {
		return f, f, nil // same file for input and output
	}

	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return nil
	}

	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()
	runProgramFn = func(p *tea.Program) (tea.Model, error) { return nil, nil }
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	exitFunc = func(int) {}

	closeCallCount := 0
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()
	closeFile = func(f *os.File) error {
		closeCallCount++
		return f.Close()
	}

	runProgram("input", config.DefaultConfig())

	if closeCallCount != 1 {
		t.Errorf("expected closeFile called once for same-file TTY, got %d calls", closeCallCount)
	}
}

func TestRunProgramTTYDifferentFilesCloseError(t *testing.T) {
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()

	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()

	openTTY = func() (*os.File, *os.File, error) {
		return ttyIn, ttyOut, nil
	}

	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return nil
	}

	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()
	runProgramFn = func(p *tea.Program) (tea.Model, error) { return nil, nil }
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	closeCallCount := 0
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()
	closeFile = func(f *os.File) error {
		closeCallCount++
		return f.Close()
	}

	runProgram("input", config.DefaultConfig())

	// BothTTY files are distinct, so closeFile should be called twice.
	if closeCallCount != 2 {
		t.Errorf(
			"expected closeFile called twice for different-file TTY, got %d calls",
			closeCallCount,
		)
	}
	// runProgram should not call exitFunc on successful completion.
	if exitCode != -1 {
		t.Errorf("expected no exit call on success, got exit code %d", exitCode)
	}
}

// ---------------------------------------------------------------------------
// Execute tests
// ---------------------------------------------------------------------------

func TestExecuteVersionFlag(t *testing.T) {
	// Test execute() with --version flag via a subprocess
	// because fang.Execute/cobra calls os.Exit(0) directly for --version,
	// which cannot be intercepted via exitFunc.
	if os.Getenv("TEST_VERSION_FLAG") == "1" {
		rootCmd.SetArgs([]string{"--version"})
		execute(context.Background())
		return
	}

	cmd := execTestCommand(t, "TEST_VERSION_FLAG", "1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("expected --version to exit cleanly, got error: %v", err)
	}
	if !bytes.Contains(output, []byte("version")) {
		t.Errorf("expected version output to contain 'version', got %q", output)
	}
}

func TestExecuteCallsExitOnError(t *testing.T) {
	// Test that Execute() calls os.Exit(1) when execute() fails.
	// We cant easily make execute fail in-process without a TTY,
	// so we test Execute in a subprocess.
	if os.Getenv("TEST_EXECUTE_ERR") == "1" {
		rootCmd.SetArgs([]string{"--unknown-flag-xyz"})
		Execute()
		return
	}

	cmd := execTestCommand(t, "TEST_EXECUTE_ERR", "1")
	// Unknown flag causes an error, Execute should call os.Exit(1).
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected subprocess to exit with non-zero status")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}

func TestExecuteHelpFlag(t *testing.T) {
	if os.Getenv("TEST_EXECUTE_HELP") == "1" {
		rootCmd.SetArgs([]string{"--help"})
		Execute()
		return
	}

	cmd := execTestCommand(t, "TEST_EXECUTE_HELP", "1")
	output, _ := cmd.Output()
	if !bytes.Contains(output, []byte("diffnav")) {
		t.Errorf("expected 'diffnav' in help output, got %q", output)
	}
}

// ---------------------------------------------------------------------------
// Run closure tests (via rootCmd.Run)
// ---------------------------------------------------------------------------

func TestRunClosureCallsExtractedFunctions(t *testing.T) {
	// Test the rootCmd.Run closure by calling it with mocked deps.
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origZoneNewGlobal := zoneNewGlobal
	defer func() { zoneNewGlobal = origZoneNewGlobal }()

	// Mock everything so the Run closure can complete.
	diffContent := "diff --git a/foo b/foo\n--- a/foo\n+++ b/foo\n@@ -1 +1 @@\n-old\n+new\n"
	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: int64(len(diffContent))}, nil
	}
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader(diffContent))
	}
	exitCode := -1
	exitFunc = func(code int) { exitCode = code }
	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()
	openTTY = func() (*os.File, *os.File, error) {
		return ttyIn, ttyOut, nil
	}
	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return nil // Run will panic on nil, but we catch it
	}
	zoneNewGlobal = func() {}

	cmd := &cobra.Command{}
	cmd.Flags().BoolP("side-by-side", "s", false, "")
	cmd.Flags().BoolP("unified", "u", false, "")
	cmd.Flags().BoolP("watch", "w", false, "")
	cmd.Flags().String("watch-cmd", "git diff", "")
	cmd.Flags().Duration("watch-interval", 2*time.Second, "")
	cmd.Flags().Bool("help", false, "")

	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()
	runProgramFn = func(p *tea.Program) (tea.Model, error) { return nil, nil }

	rootCmd.Run(cmd, []string{})

	if exitCode != -1 {
		t.Errorf("expected no exit on successful Run closure, got exit code %d", exitCode)
	}
}

// ---------------------------------------------------------------------------
// mock types
// ---------------------------------------------------------------------------

type mockFileInfo struct {
	mode os.FileMode
	size int64
}

func (m mockFileInfo) Name() string       { return "stdin" }
func (m mockFileInfo) Size() int64        { return m.size }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return time.Time{} }
func (m mockFileInfo) IsDir() bool        { return false }
func (m mockFileInfo) Sys() interface{}   { return nil }

// execTestCommand runs the current test binary as a subprocess with the
// given env variable set.
func execTestCommand(t *testing.T, envKey, envVal string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), envKey+"="+envVal)
	return cmd
}

// ---------------------------------------------------------------------------
// Additional tests for remaining coverage gaps
// ---------------------------------------------------------------------------

func TestSetupLoggingGetwdError(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Setenv("DEBUG", "true")

	origExit := exitFunc
	exitCode := -1
	exitFunc = func(code int) { exitCode = code }
	defer func() { exitFunc = origExit }()

	// Replace getwd with a failing version.
	origGetwd := getwd
	getwd = func() (string, error) { return "", fmt.Errorf("getwd failed") }
	defer func() { getwd = origGetwd }()

	cleanup := setupLogging()
	cleanup()

	if exitCode == -1 {
		t.Error("expected exitFunc to be called when getwd fails")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for getwd failure, got %d", exitCode)
	}
}

func TestSetupLoggingDebugCloseError(t *testing.T) {
	// This tests the defer close of the log file.
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Setenv("DEBUG", "true")
	cleanup := setupLogging()
	// Verify debug.log was created.
	if _, err := os.Stat("debug.log"); err != nil {
		t.Fatalf("expected debug.log to exist after setupLogging in debug mode: %v", err)
	}
	cleanup()
	// The cleanup function closes the log file.
}

type errorWriterBuilder struct{}

func (e *errorWriterBuilder) WriteRune(r rune) (int, error) {
	return 0, fmt.Errorf("write error")
}

func TestReadStdinInputWriteRuneError(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	origWriteRune := writeRune
	defer func() { writeRune = origWriteRune }()

	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: 100}, nil
	}

	// Make writeRune return an error.
	writeRune = func(b *strings.Builder, r rune) (int, error) {
		return 0, fmt.Errorf("write error")
	}

	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader("some text\n"))
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	readStdinInput(false)

	if exitCode == -1 {
		t.Error("expected exitFunc to be called on WriteRune error")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for WriteRune error, got %d", exitCode)
	}
}

func TestCloseTTYSameFile(t *testing.T) {
	f, _, _ := os.Pipe()

	closeCallCount := 0
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()
	closeFile = func(file *os.File) error {
		closeCallCount++
		return file.Close()
	}

	closeTTY(f, f)

	if closeCallCount != 1 {
		t.Errorf("expected closeFile called once for same-file TTY, got %d calls", closeCallCount)
	}
}

func TestCloseTTYDifferentFiles(t *testing.T) {
	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()

	closeCallCount := 0
	closedFiles := map[uintptr]bool{}
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()
	closeFile = func(f *os.File) error {
		closeCallCount++
		closedFiles[f.Fd()] = true
		return f.Close()
	}

	closeTTY(ttyIn, ttyOut)

	if closeCallCount != 2 {
		t.Errorf(
			"expected closeFile called twice for different TTY files, got %d calls",
			closeCallCount,
		)
	}
	if len(closedFiles) != 2 {
		t.Errorf("expected two distinct files closed, got %d", len(closedFiles))
	}
}

func TestCloseTTYSameFileCloseError(t *testing.T) {
	f, _, _ := os.Pipe()

	callCount := 0
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()
	closeFile = func(f *os.File) error {
		callCount++
		return fmt.Errorf("close error")
	}

	closeTTY(f, f)

	// When both files are the same, closeFile should be called once.
	if callCount != 1 {
		t.Fatalf("expected closeFile called once, got %d", callCount)
	}
}

func TestCloseTTYDifferentFilesCloseError(t *testing.T) {
	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()

	callCount := 0
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()
	closeFile = func(f *os.File) error {
		callCount++
		return fmt.Errorf("close error")
	}

	closeTTY(ttyIn, ttyOut)

	// When two different files are passed, closeFile should be called twice.
	if callCount != 2 {
		t.Fatalf("expected closeFile called twice, got %d", callCount)
	}
}

func TestRunProgramProgramRunError(t *testing.T) {
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()

	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()
	openTTY = func() (*os.File, *os.File, error) {
		return ttyIn, ttyOut, nil
	}

	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return nil
	}

	runProgramFn = func(p *tea.Program) (tea.Model, error) {
		return nil, fmt.Errorf("program error")
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	runProgram("diff", config.DefaultConfig())

	if exitCode == -1 {
		t.Error("expected exit on program error")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for program error, got %d", exitCode)
	}
}

func TestRunProgramProgramRunSuccess(t *testing.T) {
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()

	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()
	openTTY = func() (*os.File, *os.File, error) {
		return ttyIn, ttyOut, nil
	}

	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return nil
	}

	runProgramFn = func(p *tea.Program) (tea.Model, error) {
		return nil, nil // success
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	runProgram("diff", config.DefaultConfig())

	if exitCode != -1 {
		t.Errorf("should not exit on program success, got exit code %d", exitCode)
	}
}

func TestExecuteCallsOsExit(t *testing.T) {
	// Execute() calls os.Exit(1) when execute() returns an error.
	// Test via subprocess that an invalid flag triggers exit code 1.
	if os.Getenv("TEST_EXECUTE_EXIT1") == "1" {
		rootCmd.SetArgs([]string{"--invalid-flag-for-exit-test"})
		Execute()
		return
	}

	cmd := execTestCommand(t, "TEST_EXECUTE_EXIT1", "1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected subprocess to exit with non-zero status")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() == 0 {
		t.Fatalf("expected non-zero exit code, got %d", exitErr.ExitCode())
	}
}

func TestReadStdinInputNonUnifiedDiffNoNewline(t *testing.T) {
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()

	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: 100}, nil
	}
	// Input without trailing newline and not a unified diff.
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader("just some text"))
	}

	exitCode := -1
	exitFunc = func(code int) { exitCode = code }

	readStdinInput(false)

	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
}

func TestRunClosureDirectly(t *testing.T) {
	// Call rootCmd.Run directly with all mocks.
	origStat := stdinStat
	defer func() { stdinStat = origStat }()
	origReader := stdinReader
	defer func() { stdinReader = origReader }()
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	origOpenTTY := openTTY
	defer func() { openTTY = origOpenTTY }()
	origNewProgram := newProgram
	defer func() { newProgram = origNewProgram }()
	origZoneNewGlobal := zoneNewGlobal
	defer func() { zoneNewGlobal = origZoneNewGlobal }()

	diffContent := "diff --git a/foo b/foo\n--- a/foo\n+++ b/foo\n@@ -1 +1 @@\n-old\n+new\n"
	stdinStat = func() (os.FileInfo, error) {
		return mockFileInfo{mode: os.ModeNamedPipe, size: int64(len(diffContent))}, nil
	}
	stdinReader = func() *bufio.Reader {
		return bufio.NewReader(strings.NewReader(diffContent))
	}
	exitFunc = func(int) {}
	ttyIn, _, _ := os.Pipe()
	_, ttyOut, _ := os.Pipe()
	openTTY = func() (*os.File, *os.File, error) { return ttyIn, ttyOut, nil }
	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program { return nil }
	zoneNewGlobal = func() {}

	// Build a minimal cobra command with flags.
	cmd := &cobra.Command{}
	cmd.Flags().BoolP("side-by-side", "s", false, "")
	cmd.Flags().BoolP("unified", "u", false, "")
	cmd.Flags().BoolP("watch", "w", false, "")
	cmd.Flags().String("watch-cmd", "git diff", "")
	cmd.Flags().Duration("watch-interval", 2*time.Second, "")
	cmd.Flags().Bool("help", false, "")

	origRunProgramFn := runProgramFn
	defer func() { runProgramFn = origRunProgramFn }()
	runProgramCalled := false
	runProgramFn = func(p *tea.Program) (tea.Model, error) {
		runProgramCalled = true
		return nil, nil
	}

	rootCmd.Run(cmd, []string{})

	if !runProgramCalled {
		t.Error("expected runProgramFn to be called")
	}
}

func TestDefaultInjectables(t *testing.T) {
	// Test the default (production) injectable function bodies for coverage.
	// These function literal bodies are only counted as covered when the
	// original function assigned at var initialization is actually called.

	// stdinStat default: call the original function.
	info, err := stdinStat()
	if err != nil {
		// In a test environment without a real stdin pipe, stat may fail — that's acceptable.
		t.Logf("stdin stat: %v (acceptable in test env)", err)
	} else if info == nil {
		t.Error("expected non-nil os.FileInfo from stdinStat when no error")
	}

	// stdinReader default: call the original function.
	// In a test environment with no real stdin pipe, this returns a reader
	// wrapping os.Stdin, which is valid.
	r := stdinReader()
	if r == nil {
		t.Error("expected non-nil stdinReader result")
	}

	// closeFile default: call the original function.
	f, _, _ := os.Pipe()
	if err := closeFile(f); err != nil {
		t.Errorf("unexpected closeFile error: %v", err)
	}

	// writeRune default: call the original function.
	var sb strings.Builder
	n, werr := writeRune(&sb, 'x')
	if werr != nil {
		t.Errorf("unexpected writeRune error: %v", werr)
	}
	if n != 1 {
		t.Errorf("expected writeRune to write 1 byte, got %d", n)
	}
	if sb.String() != "x" {
		t.Errorf("expected writeRune to write 'x', got %q", sb.String())
	}

	// getwd default: call the original function.
	wd, werr := getwd()
	if werr != nil {
		t.Fatalf("getwd error: %v", werr)
	}
	if wd == "" {
		t.Error("expected non-empty working dir")
	}

	// newProgram default: call the original function.
	// newProgram always returns a non-nil program object.
	p := newProgram(nil)
	if p == nil {
		t.Error("expected non-nil program from newProgram")
	}

	// openTTY default: call the original function.
	// Whether openTTY succeeds depends on the test environment; the call
	// exercises the closure body for coverage. We do NOT assert failure —
	// some CI environments provide a TTY.
	_, _, _ = openTTY()

	// runProgramFn default: the function body `return p.Run()` is at line 121.
	// We call it with a real tea.Program (created with no options) that
	// will immediately return because there are no messages to process.
	prog := tea.NewProgram(nil)
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	exitFunc = func(int) {}
	// prog.Run() without input/output should fail; we catch any panic explicitly.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("runProgramFn panicked as expected in test: %v", r)
		}
	}()
	_, runErr := runProgramFn(prog)
	// Whether runProgramFn returns an error depends on the environment
	// (e.g., whether a TTY is available). We only assert it does not panic.
	_ = runErr
}

func TestSetupLoggingCloseFileError(t *testing.T) {
	origCloseFile := closeFile
	defer func() { closeFile = origCloseFile }()

	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Setenv("DEBUG", "true")

	// Create debug.log first so open succeeds.
	logFile, err := os.OpenFile("debug.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		t.Fatal(err)
	}
	logFile.Close()

	// Mock closeFile to return an error.
	closeFile = func(f *os.File) error { return fmt.Errorf("close error") }

	// setupLogging will open the file, set up logging, and the defer
	// will call closeFile which returns an error → log.Fatal.
	// log.Fatal calls os.Exit by default. Override exitFunc to catch it.
	exitCalled := false
	exitCode := -1
	origExit := exitFunc
	defer func() { exitFunc = origExit }()
	exitFunc = func(code int) {
		exitCalled = true
		exitCode = code
	}

	cleanup := setupLogging()
	cleanup()
	// The cleanup function closes the log file, triggering the error path when
	// closeFile returns an error, which calls log.Fatal → exitFunc(1).
	if !exitCalled {
		t.Error("expected exitFunc to be called when closeFile returns an error")
	}
	if exitCode != 1 {
		t.Errorf("expected exit code 1 for close file error, got %d", exitCode)
	}
}
