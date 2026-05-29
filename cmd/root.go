package cmd

import (
	"bufio"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	tea "charm.land/bubbletea/v2"
	"charm.land/fang/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"

	"github.com/lukaszcz/diffnav-extra/pkg/config"
	"github.com/lukaszcz/diffnav-extra/pkg/ui"
	"github.com/lukaszcz/diffnav-extra/pkg/version"
	"github.com/lukaszcz/diffnav-extra/pkg/watch"
)

//go:embed logo-diff-part.txt
var asciiArtDiffPart string

//go:embed logo-nav-part.txt
var asciiArtNavPart string

var logo = lipgloss.JoinHorizontal(lipgloss.Top,
	lipgloss.NewStyle().Foreground(lipgloss.Green).Render(asciiArtDiffPart),
	lipgloss.NewStyle().Foreground(lipgloss.Red).Render(asciiArtNavPart))

var rootCmd = &cobra.Command{
	Use:   "diffnav",
	Short: "DIFFNAV - a git diff pager based on delta but with a file tree, à la GitHub.",
	Long: "\n" + logo + lipgloss.NewStyle().Foreground(lipgloss.White).Render(
		"\na git diff pager based on delta\nbut with a file tree, à la GitHub"),
	Example: `# pipe into diffnav
git diff | diffnav

# use with the GitHub CLI
gh pr diff https://github.com/dlvhdr/gh-dash/pull/447 | diffnav

# set up as the global git diff pager
git config --global pager.diff diffnav

# watch mode: auto-refresh a diff command
diffnav --watch
diffnav --watch --watch-cmd "git diff HEAD" --watch-interval 5s
	`,
}

// execute is the core logic of Execute, extracted for testability.
func execute(ctx context.Context) error {
	themeFunc := fang.WithColorSchemeFunc(func(
		ld lipgloss.LightDarkFunc,
	) fang.ColorScheme {
		def := fang.DefaultColorScheme(ld)
		def.DimmedArgument = ld(lipgloss.Black, lipgloss.White)
		def.Codeblock = ld(lipgloss.Color("#F1EFEF"), lipgloss.Color("#141417"))
		def.Title = lipgloss.Red
		def.Command = lipgloss.Green
		def.Program = lipgloss.Green
		return def
	})

	return fang.Execute(
		ctx,
		rootCmd,
		themeFunc,
		fang.WithVersion(version.Version),
		fang.WithNotifySignal(os.Interrupt),
	)
}

func Execute() {
	if err := execute(context.Background()); err != nil {
		os.Exit(1)
	}
}

// Injectable dependencies for testing. Package-level vars allow tests to
// replace side-effecting operations without modifying the production code.
var (
	// openTTY opens a terminal device for tea. Defaults to tea.OpenTTY.
	openTTY = func() (*os.File, *os.File, error) { return tea.OpenTTY() }
	// runWatchCmd executes the watch command and returns its output. Defaults
	// to watch.RunCmd.
	runWatchCmd = watch.RunCmd
	// stdinStat returns the stdin file info. Defaults to os.Stdin.Stat.
	stdinStat = func() (os.FileInfo, error) { return os.Stdin.Stat() }
	// stdinReader returns a reader for stdin. Defaults to bufio.NewReader(os.Stdin).
	stdinReader = func() *bufio.Reader { return bufio.NewReader(os.Stdin) }
	// writeRune writes a rune to a builder. Defaults to (*strings.Builder).WriteRune.
	// Injectable for testing error paths.
	writeRune = func(b *strings.Builder, r rune) (int, error) { return b.WriteRune(r) }
	// exitFunc is called to terminate the process. Defaults to os.Exit.
	// In tests, this is replaced with a panic so recover can catch it.
	exitFunc = os.Exit
	// zoneNewGlobal initializes the bubblezone global registry. Defaults to
	// zone.NewGlobal.
	zoneNewGlobal = zone.NewGlobal
	// getwd returns the current working directory. Defaults to os.Getwd.
	getwd = os.Getwd
	// closeFile closes a file. Defaults to (*os.File).Close.
	// Injectable for testing error paths.
	closeFile = func(f *os.File) error { return f.Close() }
	// newProgram creates a bubbletea program. Defaults to tea.NewProgram.
	newProgram = func(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
		return tea.NewProgram(m, opts...)
	}
	// runProgramFn runs a tea program. Defaults to (*tea.Program).Run.
	// Injectable for testing.
	runProgramFn = func(p *tea.Program) (tea.Model, error) { return p.Run() }
)

// parseFlags extracts all CLI flag values from the cobra command.
func parseFlags(
	cmd *cobra.Command,
) (sideBySide, unified, help, watchFlag bool, watchCmd string, watchInterval time.Duration) {
	sideBySide, _ = cmd.Flags().GetBool("side-by-side")
	unified, _ = cmd.Flags().GetBool("unified")
	help, _ = cmd.Flags().GetBool("help")
	watchFlag, _ = cmd.Flags().GetBool("watch")
	watchCmd, _ = cmd.Flags().GetString("watch-cmd")
	watchInterval, _ = cmd.Flags().GetDuration("watch-interval")
	if cmd.Flags().Changed("watch-cmd") {
		watchFlag = true
	}
	return
}

// setupLogging configures the log package based on the DEBUG environment
// variable. When DEBUG is "true", logs are written to debug.log with debug
// level. Otherwise, only fatal-level messages go to stderr.
// When debug mode is active, the returned cleanup function closes the log
// file; the caller must defer it so the file stays open for the entire
// program lifetime.
func setupLogging() func() {
	if os.Getenv("DEBUG") == "true" {
		logFile, fileErr := os.OpenFile("debug.log",
			os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666)
		if fileErr != nil {
			fmt.Println("Error opening debug.log:", fileErr)
			exitFunc(1)
			return func() {}
		}

		log.SetOutput(logFile)
		log.SetTimeFormat(time.Kitchen)
		log.SetReportCaller(true)
		log.SetLevel(log.DebugLevel)

		log.SetOutput(logFile)
		log.SetColorProfile(colorprofile.TrueColor)
		wd, err := getwd()
		if err != nil {
			fmt.Println("Error getting current working dir", err)
			exitFunc(1)
			return func() {}
		}
		log.Debug("🚀 Starting diffnav", "logFile",
			wd+string(os.PathSeparator)+logFile.Name())

		return func() {
			if err := closeFile(logFile); err != nil {
				log.Error("failed closing log file", "err", err)
				exitFunc(1)
				return
			}
		}
	} else {
		log.SetOutput(os.Stderr)
		log.SetLevel(log.FatalLevel)
		return func() {}
	}
}

// readInput reads the diff input either from stdin or from the watch command.
// Returns the input string. Calls exitFunc if there is no input or if the
// input is not a unified diff (non-watch mode only).
func readInput(watchFlag bool, watchCmdStr string, helpFlag bool) string {
	if watchFlag {
		return readWatchInput(watchCmdStr)
	}
	return readStdinInput(helpFlag)
}

// readWatchInput reads diff content by running the watch command.
func readWatchInput(watchCmdStr string) string {
	stat, sErr := stdinStat()
	if sErr == nil && stat.Mode()&os.ModeNamedPipe != 0 {
		fmt.Fprintln(os.Stderr, "Warning: stdin input ignored in watch mode")
	}
	output, wErr := runWatchCmd(watchCmdStr)
	if wErr != nil {
		log.Warn("initial watch command failed, starting with empty diff", "err", wErr)
	}
	return output
}

// readStdinInput reads diff content from stdin.
func readStdinInput(helpFlag bool) string {
	stat, sErr := stdinStat()
	if sErr != nil {
		panic(sErr)
	}

	if !helpFlag && stat.Mode()&os.ModeNamedPipe == 0 && stat.Size() == 0 {
		fmt.Println("No diff, exiting")
		exitFunc(0)
		return ""
	}

	reader := stdinReader()
	var b strings.Builder

	for {
		r, _, rErr := reader.ReadRune()
		if rErr != nil && rErr == io.EOF {
			break
		}
		_, rErr = writeRune(&b, r)
		if rErr != nil {
			fmt.Println("Error getting input:", rErr)
			exitFunc(1)
			return ""
		}
	}

	input := ansi.Strip(b.String())
	if strings.TrimSpace(input) == "" {
		fmt.Println("No input provided, exiting")
		exitFunc(0)
		return ""
	}

	if !isUnifiedDiff(input) {
		fmt.Print(input)
		if !strings.HasSuffix(input, "\n") {
			fmt.Println()
		}
		exitFunc(0)
		return ""
	}

	return input
}

// buildConfig loads and applies CLI flag overrides to the configuration.
func buildConfig(
	unifiedFlag, sideBySideFlag bool,
	watchFlag bool,
	watchCmdStr string,
	watchInterval time.Duration,
) config.Config {
	cfg := config.Load()

	if unifiedFlag {
		cfg.UI.SideBySide = false
	} else if sideBySideFlag {
		cfg.UI.SideBySide = true
	}

	cfg.Watch = config.WatchConfig{
		Enabled:  watchFlag,
		Cmd:      watchCmdStr,
		Interval: watchInterval,
	}

	return cfg
}

// runProgram opens the TTY, creates a bubbletea program, and runs it.
func runProgram(input string, cfg config.Config) {
	ttyIn, ttyOut, err := openTTY()
	if err != nil {
		log.Error("failed to open tty", "err", err)
		exitFunc(1)
		return
	}
	defer closeTTY(ttyIn, ttyOut)

	p := newProgram(ui.New(input, cfg), tea.WithInput(ttyIn), tea.WithOutput(ttyOut))

	if _, err := runProgramFn(p); err != nil {
		log.Error("program error", "err", err)
		exitFunc(1)
		return
	}
}

// closeTTY closes the TTY file handles, joining all errors.
func closeTTY(ttyIn, ttyOut *os.File) {
	var closeErr error
	if ttyOut == ttyIn {
		if err := closeFile(ttyIn); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close tty: %w", err))
		}
	} else {
		if err := closeFile(ttyIn); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close tty input: %w", err))
		}
		if err := closeFile(ttyOut); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close tty output: %w", err))
		}
	}
	if closeErr != nil {
		log.Printf("tty cleanup error: %v", closeErr)
	}
}

func init() {
	rootCmd.Flags().BoolP("side-by-side", "s", false, "Force side-by-side diff view")

	rootCmd.Flags().BoolP("unified", "u", false, "Force unified diff view")

	rootCmd.Flags().
		BoolP("watch", "w", false, "Watch mode: periodically re-run a diff command and refresh")
	rootCmd.Flags().String("watch-cmd", "git diff", "Command to run in watch mode")
	rootCmd.Flags().Duration("watch-interval", 2*time.Second, "Interval between watch refreshes")

	rootCmd.SetVersionTemplate("\n" + logo + "\n" + `{{printf "version %s\n" .Version}}`)

	rootCmd.Run = func(cmd *cobra.Command, args []string) {
		sideBySideFlag, unifiedFlag, helpFlag, watchFlag, watchCmdStr, watchInterval := parseFlags(
			cmd,
		)

		zoneNewGlobal()

		defer setupLogging()()

		input := readInput(watchFlag, watchCmdStr, helpFlag)

		cfg := buildConfig(unifiedFlag, sideBySideFlag, watchFlag, watchCmdStr, watchInterval)

		runProgram(input, cfg)
	}
}
