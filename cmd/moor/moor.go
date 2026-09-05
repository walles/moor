// Command line launcher for moor
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	log "github.com/sirupsen/logrus"
	"golang.org/x/term"

	"github.com/walles/moor/v2/internal"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/internal/util"
	"github.com/walles/twin"
)

var versionString = ""

// Which environment variable should we get our config from?
//
// Prefer MOOR, but if that's not set, look at MOAR as well for backwards
// compatibility reasons.
func moorEnvVarName() string {
	moorEnvSet := len(strings.TrimSpace(os.Getenv("MOOR"))) > 0
	if moorEnvSet {
		return "MOOR"
	}

	moarEnvSet := len(strings.TrimSpace(os.Getenv("MOAR"))) > 0
	if moarEnvSet {
		// Legacy, keep for backwards compatibility
		return "MOAR"
	}

	// This is the default
	return "MOOR"
}

// printProblemsHeader prints bug reporting information to stderr
func printProblemsHeader() {
	fmt.Fprintln(os.Stderr, "Please post the following report at <https://github.com/walles/moor/issues>,")
	fmt.Fprintln(os.Stderr, "or e-mail it to johan.walles@gmail.com.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Version       :", getVersion())
	fmt.Fprintln(os.Stderr, "LANG          :", os.Getenv("LANG"))
	fmt.Fprintln(os.Stderr, "TERM          :", os.Getenv("TERM"))
	fmt.Fprintln(os.Stderr, "EDITOR        :", os.Getenv("EDITOR"))
	fmt.Fprintln(os.Stderr, "TERM_PROGRAM  :", os.Getenv("TERM_PROGRAM"))
	if value := os.Getenv("PAGER_WRAP_COLUMNS"); value != "" {
		fmt.Fprintln(os.Stderr, "PAGER_WRAP_COLUMNS :", value)
	}

	fmt.Fprintln(os.Stderr)

	lessenv_section := ""
	lessTermcapVars := []string{
		"LESS_TERMCAP_md",
		"LESS_TERMCAP_us",
		"LESS_TERMCAP_so",
	}
	for _, varName := range lessTermcapVars {
		value := os.Getenv(varName)
		if value == "" {
			continue
		}

		lessenv_section += varName + " : " + strings.ReplaceAll(value, "\x1b", "ESC") + "\n"
	}
	if lessenv_section != "" {
		fmt.Fprintln(os.Stderr, lessenv_section)
	}

	fmt.Fprintln(os.Stderr, "GOOS    :", runtime.GOOS)
	fmt.Fprintln(os.Stderr, "GOARCH  :", runtime.GOARCH)
	fmt.Fprintln(os.Stderr, "Compiler:", runtime.Compiler)
	fmt.Fprintln(os.Stderr, "NumCPU  :", runtime.NumCPU())
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Stdin  is a terminal:", term.IsTerminal(int(os.Stdin.Fd())))
	fmt.Fprintln(os.Stderr, "Stdout is a terminal:", term.IsTerminal(int(os.Stdout.Fd())))
	fmt.Fprintln(os.Stderr)

	if moorEnvVarName() == "MOAR" {
		fmt.Fprintln(os.Stderr, "MOAR (legacy):", os.Getenv("MOAR"))
	} else {
		fmt.Fprintln(os.Stderr, "MOOR:", os.Getenv("MOOR"))
	}
	fmt.Fprintf(os.Stderr, "Commandline: %#v\n", os.Args)
}

func parseLexerOption(lexerOption string) (chroma.Lexer, error) {
	byMimeType := lexers.MatchMimeType(lexerOption)
	if byMimeType != nil {
		return byMimeType, nil
	}

	// Use Chroma's built-in fuzzy lexer picker
	lexer := lexers.Get(lexerOption)
	if lexer != nil {
		return lexer, nil
	}

	return nil, fmt.Errorf(
		"Look here for inspiration: https://github.com/alecthomas/chroma/tree/master/lexers/embedded",
	)
}

func parseStyleOption(styleOption string) (*chroma.Style, error) {
	style, ok := styles.Registry[styleOption]
	if !ok {
		return &chroma.Style{}, fmt.Errorf(
			"Pick a style from here: https://xyproto.github.io/splash/docs/longer/all.html")
	}

	return style, nil
}

func parseColorsOption(colorsOption string) (twin.ColorCount, error) {
	if strings.ToLower(colorsOption) == "auto" {
		colorsOption = "16M"
		if os.Getenv("COLORTERM") != "truecolor" && strings.Contains(os.Getenv("TERM"), "256") {
			// Covers "xterm-256color" as used by the macOS Terminal
			colorsOption = "256"
		}
	}

	switch strings.ToUpper(colorsOption) {
	case "8":
		return twin.ColorCount8, nil
	case "16":
		return twin.ColorCount16, nil
	case "256":
		return twin.ColorCount256, nil
	case "16M":
		return twin.ColorCount24bit, nil
	}

	var noColor twin.ColorCount
	return noColor, fmt.Errorf("Valid counts are 8, 16, 256, 16M or auto")
}

func parseStatusBarStyle(styleOption string) (internal.StatusBarOption, error) {
	if styleOption == "inverse" {
		return internal.STATUSBAR_STYLE_INVERSE, nil
	}
	if styleOption == "plain" {
		return internal.STATUSBAR_STYLE_PLAIN, nil
	}
	if styleOption == "bold" {
		return internal.STATUSBAR_STYLE_BOLD, nil
	}

	return 0, fmt.Errorf("Good ones are inverse, plain and bold")
}

func parseUnprintableStyle(styleOption string) (textstyles.UnprintableStyleT, error) {
	if styleOption == "highlight" {
		return textstyles.UnprintableStyleHighlight, nil
	}
	if styleOption == "whitespace" {
		return textstyles.UnprintableStyleWhitespace, nil
	}

	return 0, fmt.Errorf("Good ones are highlight or whitespace")
}

func parseScrollHint(scrollHint string) (textstyles.CellWithMetadata, error) {
	scrollHint = strings.ReplaceAll(scrollHint, "ESC", "\x1b")

	parsedTokens := textstyles.StyledRunesFromString(twin.StyleDefault, scrollHint, nil, 0).StyledRunes
	if len(parsedTokens) == 1 {
		return parsedTokens[0], nil
	}

	return textstyles.CellWithMetadata{}, fmt.Errorf("Expected exactly one (optionally highlighted) character. For example: 'ESC[2m…'")
}

func parseShiftAmount(shiftAmount string) (uint, error) {
	value, err := strconv.ParseUint(shiftAmount, 10, 32)
	if err != nil {
		return 0, err
	}

	if value < 1 {
		return 0, fmt.Errorf("Shift amount must be at least 1")
	}

	// Let's add an upper bound as well if / when requested

	return uint(value), nil
}

func parseTabAmount(tabAmount string) (uint, error) {
	value, err := strconv.ParseUint(tabAmount, 10, 32)
	if err != nil {
		return 0, err
	}

	if value < 1 {
		return 0, fmt.Errorf("Tab size must be at least 1")
	}

	// Let's add an upper bound as well if / when requested

	return uint(value), nil
}

func parseMouseMode(mouseMode string) (twin.MouseMode, error) {
	switch mouseMode {
	case "auto":
		return twin.MouseModeAuto, nil
	case "select", "mark":
		return twin.MouseModeSelect, nil
	case "scroll":
		return twin.MouseModeScroll, nil
	}

	return twin.MouseModeAuto, fmt.Errorf("Valid modes are auto, select and scroll")
}

func pumpToStdout(inputFilenames ...string) error {
	if len(inputFilenames) > 0 {
		stdinDone := false

		// If we get both redirected stdin and an input filenames, should only
		// copy the files and ignore stdin, because that's how less works.
		for _, inputFilename := range inputFilenames {
			if inputFilename == "-" && !term.IsTerminal(int(os.Stdin.Fd())) {
				// "-" with redirected stdin means "read from stdin"
				if stdinDone {
					// stdin already drained, don't do it again
					continue
				}

				_, err := io.Copy(os.Stdout, os.Stdin)
				if err != nil {
					return fmt.Errorf("Failed to copy stdin to stdout: %w", err)
				}

				stdinDone = true
				continue
			}

			inputFile, _, err := reader.ZOpen(inputFilename)
			if err != nil {
				return fmt.Errorf("Failed to open %s: %w", inputFilename, err)
			}

			_, err = io.Copy(os.Stdout, inputFile)
			if err != nil {
				return fmt.Errorf("Failed to copy %s to stdout: %w", inputFilename, err)
			}
		}

		return nil
	}

	// No input filenames, pump stdin to stdout
	_, err := io.Copy(os.Stdout, os.Stdin)
	if err != nil {
		return fmt.Errorf("Failed to copy stdin to stdout: %w", err)
	}
	return nil
}

func russiaNotSupported() {
	if !strings.HasPrefix(strings.ToLower(os.Getenv("LANG")), "ru_ru") {
		// Not russia
		return
	}

	if os.Getenv("CRIMEA") == "Ukraine" {
		// It is!
		return
	}

	fmt.Fprintln(os.Stderr, "ERROR: russia not supported (but Russian is!)")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "Options for using moor in Russian:")
	fmt.Fprintln(os.Stderr, "* Change your language setting to ru_UA.UTF-8")
	fmt.Fprintln(os.Stderr, "* Set CRIMEA=Ukraine in your environment")
	fmt.Fprintln(os.Stderr, "* russia leaves Ukraine")
	os.Exit(1)
}

// For git output and man pages, disable line numbers by default.
//
// Before paging, "man" first checks the terminal width and formats the man page
// to fit that width. Git does that as well for git diff --stat.
//
// Then, if moor adds line numbers, the rightmost part of the page will scroll
// out of view.
//
// So we try to this, and in that case disable line numbers so that the
// rightmost part of the page is visible by default.
//
// See also internal/haveLoadedManPage(), where we try to detect man pages by
// their contents.
func noLineNumbersDefault() bool {
	if os.Getenv("MAN_PN") != "" {
		// Set by "man" on Ubuntu 22.04.4 when I tested it inside of Docker.
		log.Debug("MAN_PN is set, skipping line numbers for man page")
		return true
	}

	if os.Getenv("GIT_EXEC_PATH") != "" {
		// Set by "git".

		// Neither logs nor diffs are helped by line numbers, turn them off by
		// default.
		log.Debug("GIT_EXEC_PATH is set, skipping line numbers when paging git output")
		return true
	}

	// Default to not skipping line numbers
	return false
}

// unescapeManPn removes escaping inserted by GNU man's escape_less function.
// It escapes ? : . % \ with a backslash.
func unescapeManPn(manPn string) string {
	return strings.NewReplacer(
		`\?`, `?`,
		`\:`, `:`,
		`\.`, `.`,
		`\%`, `%`,
		`\\`, `\`,
	).Replace(manPn)
}

// Return complete version when built with build.sh or fallback to module version (i.e. "go install")
func getVersion() string {
	if versionString != "" {
		return versionString
	}
	info, ok := debug.ReadBuildInfo()
	if ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "Should be set when building, please use build.sh to build"
}

// Can return a nil pager on --help or --version, or if pumping to stdout.
func pagerFromArgs(
	args []string,
	newScreen func(mouseMode twin.MouseMode, terminalColorCount twin.ColorCount) (twin.Screen, error),
	stdinIsRedirected bool,
	stdoutIsRedirected bool,
) (
	*internal.Pager, twin.Screen, chroma.Style, *chroma.Formatter, bool, error,
) {
	// FIXME: If we get a CTRL-C, get terminal back into a useful state before terminating

	flagSet := flag.NewFlagSet("",
		flag.ContinueOnError, // We want to do our own error handling
	)
	flagSet.SetOutput(io.Discard) // We want to do our own printing

	printVersion := flagSet.Bool("version", false, "Prints the moor version number")
	debug := flagSet.Bool("debug", false, "Print debug logs after exiting")
	trace := flagSet.Bool("trace", false, "Print trace logs after exiting")

	wrap := flagSet.Bool("wrap", false, "Wrap long lines")
	follow := flagSet.Bool("follow", false, "Follow piped input just like \"tail -f\"")
	styleOption := flagSetFunc(flagSet,
		"style", nil,
		"Highlighting `style` from https://xyproto.github.io/splash/docs/longer/all.html", parseStyleOption)
	lexer := flagSetFunc(flagSet,
		"lang", nil,
		"File contents, used for highlighting. Mime type or file extension (\"html\"). Default is to guess by filename.", parseLexerOption)
	terminalFg := flagSet.Bool("terminal-fg", false, "Use terminal foreground color rather than style foreground for plain text")
	noSearchLineHighlight := flagSet.Bool("no-search-line-highlight", false, "Do not highlight the background of lines with search hits")

	defaultFormatter, err := parseColorsOption("auto")
	if err != nil {
		panic(fmt.Errorf("Failed parsing default formatter: %w", err))
	}
	terminalColorsCount := flagSetFunc(flagSet,
		"colors", defaultFormatter, "Highlighting palette size: 8, 16, 256, 16M, auto", parseColorsOption)

	noLineNumbers := flagSet.Bool("no-linenumbers", noLineNumbersDefault(), "Hide line numbers on startup, press left arrow key to show")
	noStatusBar := flagSet.Bool("no-statusbar", false, "Hide the status bar, toggle with '='")
	reFormat := flagSet.Bool("reformat", false, "Reformat some input files (JSON)")
	flagSet.Bool("no-reformat", true, "No effect, kept for compatibility. See --reformat")
	quitIfOneScreen := flagSet.Bool("quit-if-one-screen", false, "Don't page if contents fits on one screen. Affected by --no-clear-on-exit-margin.")
	noClearOnExit := flagSet.Bool("no-clear-on-exit", false, "Retain screen contents when exiting moor")
	noClearOnExitMargin := flagSet.Int("no-clear-on-exit-margin", 1,
		"Number of lines to leave for your shell prompt, defaults to 1")
	statusBarStyle := flagSetFunc(flagSet, "statusbar", internal.STATUSBAR_STYLE_INVERSE,
		"Status bar `style`: inverse, plain or bold", parseStatusBarStyle)
	unprintableStyle := flagSetFunc(flagSet, "render-unprintable", textstyles.UnprintableStyleHighlight,
		"How unprintable characters are rendered: highlight or whitespace", parseUnprintableStyle)
	scrollLeftHint := flagSetFunc(flagSet, "scroll-left-hint",
		textstyles.CellWithMetadata{Rune: '<', Style: twin.StyleDefault.WithAttr(twin.AttrReverse)},
		"Shown when view can scroll left. One character with optional ANSI highlighting.", parseScrollHint)
	scrollRightHint := flagSetFunc(flagSet, "scroll-right-hint",
		textstyles.CellWithMetadata{Rune: '>', Style: twin.StyleDefault.WithAttr(twin.AttrReverse)},
		"Shown when view can scroll right. One character with optional ANSI highlighting.", parseScrollHint)
	shift := flagSetFunc(flagSet, "shift", 16, "Horizontal scroll `amount` >=1, defaults to 16", parseShiftAmount)
	tabSize := flagSetFunc(flagSet, "tab-size", 8, "Number of spaces per tab stop, defaults to 8", parseTabAmount)
	mouseMode := flagSetFunc(
		flagSet,
		"mousemode",
		twin.MouseModeAuto,
		"Mouse `mode`: auto, select or scroll: https://github.com/walles/moor/blob/master/MOUSE.md",
		parseMouseMode,
	)

	// Combine flags from environment and from command line
	flags := args[1:]
	moorEnv := strings.TrimSpace(os.Getenv(moorEnvVarName()))
	if len(moorEnv) > 0 {
		// FIXME: It would be nice if we could debug log that we're doing this,
		// but logging is not yet set up and depends on command line parameters.
		flags = append(strings.Fields(moorEnv), flags...)
	}

	targetLine, initialSearch, remainingArgs := parsePlusArgs(flags)

	err = flagSet.Parse(remainingArgs)

	if err == nil {
		if *noClearOnExitMargin < 0 {
			err = fmt.Errorf("Invalid --no-clear-on-exit-margin %d, must be 0 or higher", *noClearOnExitMargin)
		}
	}

	if err != nil {
		if err == flag.ErrHelp {
			printUsage(flagSet, *terminalColorsCount)
			return nil, nil, chroma.Style{}, nil, false, nil
		}

		errorText := err.Error()
		if strings.HasPrefix(errorText, "invalid value") {
			errorText = strings.Replace(errorText, ": ", "\n\n", 1)
		}

		boldErrorMessage := "\x1b[1m" + errorText + "\x1b[m"
		fmt.Fprintln(os.Stderr, "ERROR:", boldErrorMessage)
		fmt.Fprintln(os.Stderr)
		printCommandline(os.Stderr)
		fmt.Fprintln(os.Stderr, "For help, run: \x1b[1mmoor --help\x1b[m")

		os.Exit(1)
	}

	logsRequested := *debug || *trace

	if *printVersion {
		fmt.Println(getVersion())
		return nil, nil, chroma.Style{}, nil, logsRequested, nil
	}

	log.SetLevel(log.InfoLevel)
	if *trace {
		log.SetLevel(log.TraceLevel)
	} else if *debug {
		log.SetLevel(log.DebugLevel)
	}

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMicro,
	})

	flagSetArgs := flagSet.Args()
	if stdinIsRedirected && len(flagSetArgs) == 0 {
		// "-" is special if stdin is redirected, means "read from stdin"
		//
		// Ref: https://github.com/walles/moor/issues/162
		flagSetArgs = []string{"-"}
	}

	// Check that any input files can be opened
	for _, inputFilename := range flagSetArgs {
		if stdinIsRedirected && inputFilename == "-" {
			// stdin can be opened
			continue
		}

		// Need to check before newScreen() below, otherwise the screen
		// will be cleared before we print the "No such file" error.
		err := reader.TryOpen(inputFilename)
		if err != nil {
			return nil, nil, chroma.Style{}, nil, logsRequested, err
		}
	}

	if len(flagSetArgs) == 0 && !stdinIsRedirected {
		fmt.Fprintln(os.Stderr, "ERROR: Filename(s) or input pipe required (\"moor file.txt\")")
		fmt.Fprintln(os.Stderr)
		printCommandline(os.Stderr)
		fmt.Fprintln(os.Stderr, "For help, run: \x1b[1mmoor --help\x1b[m")
		os.Exit(1)
	}

	if stdoutIsRedirected {
		err := pumpToStdout(flagSetArgs...)
		if err != nil {
			return nil, nil, chroma.Style{}, nil, logsRequested, err
		}
		return nil, nil, chroma.Style{}, nil, logsRequested, nil
	}

	// INVARIANT: At this point, stdout is a terminal and we should proceed with
	// paging.
	stdoutIsTerminal := !stdoutIsRedirected
	if !stdoutIsTerminal {
		panic("Invariant broken: stdout is not a terminal")
	}

	formatter := formatters.TTY256
	switch *terminalColorsCount {
	case twin.ColorCount8:
		formatter = formatters.TTY8
	case twin.ColorCount16:
		formatter = formatters.TTY16
	case twin.ColorCount24bit:
		formatter = formatters.TTY16m
	}

	var readerImpls []*reader.ReaderImpl
	shouldFormat := *reFormat
	readerOptions := reader.ReaderOptions{Lexer: *lexer, ShouldFormat: shouldFormat}

	stdinName := ""
	if os.Getenv("PAGER_LABEL") != "" {
		stdinName = os.Getenv("PAGER_LABEL")
	} else if os.Getenv("MAN_PN") != "" {
		// MAN_PN is set by GNU man. Example value: "printf(1)"
		stdinName = unescapeManPn(os.Getenv("MAN_PN"))
	}

	// Display the input file(s) contents
	stdinDone := false
	for _, inputFilename := range flagSetArgs {
		var readerImpl *reader.ReaderImpl
		var err error

		if stdinIsRedirected && inputFilename == "-" {
			if stdinDone {
				// stdin already drained, don't do it again
				continue
			}

			readerImpl, err = reader.NewFromStream(stdinName, os.Stdin, formatter, readerOptions)
			if err != nil {
				return nil, nil, chroma.Style{}, nil, logsRequested, err
			}

			// If the user is doing "sudo something | moor" we can't show the UI until
			// we start getting data, otherwise we'll mess up sudo's password prompt.
			readerImpl.AwaitFirstByte()

			stdinDone = true
		} else {
			readerImpl, err = reader.NewFromFilename(inputFilename, formatter, readerOptions)
		}

		if err != nil {
			return nil, nil, chroma.Style{}, nil, logsRequested, err
		}
		readerImpls = append(readerImpls, readerImpl)
	}

	// We got the first byte, this means sudo is done (if it was used) and we
	// can set up the UI.
	screen, err := newScreen(*mouseMode, *terminalColorsCount)
	if err != nil {
		// Ref: https://github.com/walles/moor/issues/149
		log.Info("Failed to set up screen for paging, pumping to stdout instead: ", err)

		for _, readerImpl := range readerImpls {
			readerImpl.PumpToStdout()
		}

		return nil, nil, chroma.Style{}, nil, logsRequested, nil
	}

	var style chroma.Style
	if *styleOption == nil {
		style = internal.GetStyleForScreen(screen)
	} else {
		style = **styleOption
	}
	log.Debug("Using style <", style.Name, ">")
	for _, readerImpl := range readerImpls {
		readerImpl.SetStyleForHighlighting(style)
	}

	pager := internal.NewPager(readerImpls...)
	pager.WrapLongLines = *wrap
	pager.ShowLineNumbers = !*noLineNumbers
	pager.ShowStatusBar = !*noStatusBar
	pager.DeInit = !*noClearOnExit
	pager.DeInitFalseMargin = *noClearOnExitMargin
	pager.QuitIfOneScreen = *quitIfOneScreen
	pager.StatusBarStyle = *statusBarStyle
	pager.UnprintableStyle = *unprintableStyle
	pager.WithTerminalFg = *terminalFg
	pager.ScrollLeftHint = *scrollLeftHint
	pager.ScrollRightHint = *scrollRightHint
	pager.SideScrollAmount = int(*shift)
	pager.TabSize = int(*tabSize)
	pager.WithSearchHitLineBackground = !*noSearchLineHighlight

	if value, err := strconv.Atoi(os.Getenv("PAGER_WRAP_COLUMNS")); err == nil {
		pager.Width = value
	}

	if initialSearch != nil {
		pager.InitialSearch = *initialSearch
	}

	pager.TargetLine = targetLine
	if *follow && pager.TargetLine == nil {
		pager.TargetLine = new(linemetadata.IndexMax())
	}

	return pager, screen, style, &formatter, logsRequested, nil
}

func main() {
	var loglines internal.LogWriter
	logsRequested := false
	log.SetOutput(&loglines)
	twin.SetLogger(&util.TwinLogger{})
	russiaNotSupported()

	defer func() {
		err := recover()
		haveLogsToShow := len(loglines.String()) > 0 && logsRequested
		if err == nil && !haveLogsToShow {
			// No problems
			return
		}

		printProblemsHeader()

		if len(loglines.String()) > 0 {
			fmt.Fprintln(os.Stderr)
			// Consider not printing duplicate log messages more than once
			fmt.Fprintf(os.Stderr, "%s", loglines.String())
		}

		if err != nil {
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "Panic recovery timestamp:", time.Now().String())
			fmt.Fprintln(os.Stderr)
			panic(err)
		}

		// We were asked to print logs, and we did. Success!
		os.Exit(0)
	}()

	stdinIsRedirected := !term.IsTerminal(int(os.Stdin.Fd()))
	stdoutIsRedirected := !term.IsTerminal(int(os.Stdout.Fd()))

	pager, screen, style, formatter, _logsRequested, err := pagerFromArgs(
		os.Args,
		twin.NewScreenWithMouseModeAndColorCount,
		stdinIsRedirected,
		stdoutIsRedirected,
	)
	logsRequested = _logsRequested
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}

	if pager == nil {
		// No pager, we're done
		return
	}

	startPaging(pager, screen, &style, formatter)
}

// Define a generic flag with specified name, default value, and usage string.
// The return value is the address of a variable that stores the parsed value of
// the flag.
func flagSetFunc[T any](flagSet *flag.FlagSet, name string, defaultValue T, usage string, parser func(valueString string) (T, error)) *T {
	parsed := defaultValue

	flagSet.Func(name, usage, func(valueString string) error {
		parseResult, err := parser(valueString)
		if err != nil {
			return err
		}
		parsed = parseResult
		return nil
	})

	return &parsed
}

func startPaging(pager *internal.Pager, screen twin.Screen, chromaStyle *chroma.Style, chromaFormatter *chroma.Formatter) {
	// Handle SIGINT and SIGTERM
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		defer func() {
			internal.PanicHandler("startPaging()/signals", recover(), debug.Stack())
		}()

		sig := <-signals
		log.Infof("Got signal %v, exiting...", sig)
		screen.Close()

		exitCode := 1
		syscallSignal, ok := sig.(syscall.Signal)
		if ok {
			// Ref: https://hypothesis.sh/references/exit-codes?grp=signal
			exitCode = 128 + int(syscallSignal)
		}

		os.Exit(exitCode)
	}()

	defer func() {
		panicMessage := recover()
		if panicMessage != nil {
			// Clarify that any screen shutdown logs are from panic handling,
			// not something the user or some external thing did.
			log.Info("Panic detected, closing screen before informing the user...")
		}

		// Restore screen...
		screen.Close()

		// ... before printing any panic() output, otherwise the output will
		// have broken linefeeds and be hard to follow.
		if panicMessage != nil {
			panic(panicMessage)
		}

		if !pager.DeInit {
			pager.ReprintAfterExit()
		}
	}()

	pager.StartPaging(screen, chromaStyle, chromaFormatter)
}
