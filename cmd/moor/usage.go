package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal"
	"github.com/walles/twin"
)

func renderLessTermcapEnvVar(envVarName string, description string, colors twin.ColorCount) string {
	value := os.Getenv(envVarName)
	if len(value) == 0 {
		return ""
	}

	style, err := internal.TermcapToStyle(value)
	if err != nil {
		bold := twin.StyleDefault.WithAttr(twin.AttrBold).RenderUpdateFrom(twin.StyleDefault, colors)
		notBold := twin.StyleDefault.RenderUpdateFrom(twin.StyleDefault.WithAttr(twin.AttrBold), colors)
		return fmt.Sprintf("  %s (%s): %s %s<- Error: %v%s\n",
			envVarName,
			description,
			strings.ReplaceAll(value, "\x1b", "ESC"),
			bold,
			err,
			notBold,
		)
	}

	prefix := style.RenderUpdateFrom(twin.StyleDefault, colors)
	suffix := twin.StyleDefault.RenderUpdateFrom(style, colors)
	return fmt.Sprintf("  %s (%s): %s\n",
		envVarName,
		description,
		prefix+strings.ReplaceAll(value, "\x1b", "ESC")+suffix,
	)
}

func renderPagerEnvVar(name string, colors twin.ColorCount) string {
	bold := twin.StyleDefault.WithAttr(twin.AttrBold).RenderUpdateFrom(twin.StyleDefault, colors)
	notBold := twin.StyleDefault.RenderUpdateFrom(twin.StyleDefault.WithAttr(twin.AttrBold), colors)

	value, isSet := os.LookupEnv(name)
	if value == "" {
		what := "unset"
		if isSet {
			what = "empty"
		}

		return fmt.Sprintf("  %s is %s %s<- Should be %s%s\n",
			name,
			what,
			bold,
			getMoorPath(),
			notBold,
		)
	}

	absMoorPath, err := absLookPath(os.Args[0])
	if err != nil {
		log.Warn("Unable to find absolute moor path: ", err)
		return ""
	}

	// You can actually have command line arguments in these variables (like
	// MANPAGER="moor --reformat"), so we want to look at only the first word to
	// find out whether this PAGER variable is pointing to the right binary.
	firstWord := strings.Fields(value)[0]
	absEnvValue, err := absLookPath(firstWord)
	if err != nil {
		// This can happen if this is set to some outdated value
		absEnvValue = value
	}

	if absEnvValue == absMoorPath {
		return fmt.Sprintf("  %s=%s\n", name, value)
	}

	return fmt.Sprintf("  %s=%s %s<- Should be %s%s\n",
		name,
		value,
		bold,
		getMoorPath(),
		notBold,
	)
}

// If the environment variable is set, render it as APA=bepa indented two
// spaces, plus a newline at the end. Otherwise, return an empty string.
func renderPlainEnvVar(envVarName string) string {
	value := os.Getenv(envVarName)
	if value == "" {
		return ""
	}

	return fmt.Sprintf("  %s=%s\n", envVarName, value)
}

func printCommandline(output io.Writer) {
	envVarName := moorEnvVarName()
	envVarDescription := envVarName
	if envVarName != "MOOR" {
		bold := twin.StyleDefault.WithAttr(twin.AttrBold).RenderUpdateFrom(twin.StyleDefault, twin.ColorCount256)
		notBold := twin.StyleDefault.RenderUpdateFrom(twin.StyleDefault.WithAttr(twin.AttrBold), twin.ColorCount256)

		envVarDescription = envVarName + " (" + bold + "legacy, please use MOOR instead!" + notBold + ")"
	}

	fmt.Fprintln(output, "Commandline: moor", strings.Join(os.Args[1:], " "))                 //nolint:errcheck
	fmt.Fprintf(output, "Environment: %s=\"%v\"\n", envVarDescription, os.Getenv(envVarName)) //nolint:errcheck
	fmt.Fprintln(output)                                                                      //nolint:errcheck
}

func heading(text string, colors twin.ColorCount) string {
	style := twin.StyleDefault.WithAttr(twin.AttrItalic)
	prefix := style.RenderUpdateFrom(twin.StyleDefault, colors)
	suffix := twin.StyleDefault.RenderUpdateFrom(style, colors)
	return prefix + text + suffix
}

func printUsage(flagSet *flag.FlagSet, colors twin.ColorCount) {
	// This controls where PrintDefaults() prints, see below
	flagSet.SetOutput(os.Stdout)

	// FIXME: Log if any printouts fail?

	fmt.Println(heading("Usage", colors))
	fmt.Println("  moor [options] <file>...")
	fmt.Println("  ... | moor")
	fmt.Println("  moor < file")
	fmt.Println()
	fmt.Println("Shows file contents. Compressed files will be transparently decompressed.")
	fmt.Println("Input is expected to be (possibly compressed) UTF-8 encoded text. Invalid /")
	fmt.Println("non-printable characters are by default rendered as '?'. Press : inside of")
	fmt.Println("moor to switch between files.")
	fmt.Println()
	fmt.Println("More information + source code:")
	fmt.Println("  <https://github.com/walles/moor#readme>")
	fmt.Println()
	fmt.Println(heading("Environment", colors))

	envVarName := moorEnvVarName()
	envVarValue := os.Getenv(envVarName)

	if len(envVarValue) == 0 {
		fmt.Println("  Additional options are read from the MOOR environment variable if set.")
		fmt.Println("  But currently, the MOOR environment variable is not set.")
	} else {
		fmt.Printf("  Additional options are read from the %s environment variable.\n", envVarName)
		if envVarName != "MOOR" {
			bold := twin.StyleDefault.WithAttr(twin.AttrBold).RenderUpdateFrom(twin.StyleDefault, colors)
			notBold := twin.StyleDefault.RenderUpdateFrom(twin.StyleDefault.WithAttr(twin.AttrBold), colors)

			fmt.Printf(
				"  But that is going away, %splease use the MOOR environment variable instead%s!\n",
				bold,
				notBold)
		}
		fmt.Printf("  Current setting: %s=\"%s\"\n", envVarName, envVarValue)
	}

	envSection := ""
	envSection += renderLessTermcapEnvVar("LESS_TERMCAP_md", "man page bold style", colors)
	envSection += renderLessTermcapEnvVar("LESS_TERMCAP_us", "man page underline style", colors)
	envSection += renderLessTermcapEnvVar("LESS_TERMCAP_so", "search hits and footer style", colors)

	envSection += renderPagerEnvVar("PAGER", colors)
	envVars := os.Environ()
	sort.Strings(envVars)
	for _, env := range envVars {
		split := strings.SplitN(env, "=", 2)
		if len(split) != 2 {
			continue
		}

		name := split[0]
		if name == "PAGER" {
			// Already done above
			continue
		}
		if !strings.HasSuffix(name, "PAGER") {
			continue
		}

		envSection += renderPagerEnvVar(name, colors)
	}

	envSection += renderPlainEnvVar("TERM")
	envSection += renderPlainEnvVar("TERM_PROGRAM")
	envSection += renderPlainEnvVar("COLORTERM")

	// Requested here: https://github.com/walles/moor/issues/170#issuecomment-1891154661
	envSection += renderPlainEnvVar("MANROFFOPT")

	if envSection != "" {
		fmt.Println()

		// Not Println since the section already ends with a newline
		fmt.Print(envSection)
	}

	printSetDefaultPagerHelp(colors)

	fmt.Println()
	fmt.Println(heading("Options", colors))

	flagSet.PrintDefaults()

	fmt.Println("  +1234")
	fmt.Println("    \tImmediately scroll to line 1234")
	fmt.Println("  +/pattern")
	fmt.Println("    \tImmediately search for \"pattern\"")
}

// If $PAGER isn't pointing to us, print a help text on how to set it.
func printSetDefaultPagerHelp(colors twin.ColorCount) {
	absMoorPath, err := absLookPath(os.Args[0])
	if err != nil {
		log.Warn("Unable to find moor binary ", err)
		return
	}

	absPagerValue, err := absLookPath(os.Getenv("PAGER"))
	if err != nil {
		absPagerValue = ""
	}

	if absPagerValue == absMoorPath {
		// We're already the default pager
		return
	}

	fmt.Println()
	fmt.Println(heading("Making moor Your Default Pager", colors))

	shellIsFish := strings.HasSuffix(os.Getenv("SHELL"), "fish")
	shellIsPowershell := len(os.Getenv("PSModulePath")) > 0

	if shellIsFish {
		fmt.Println("  Write this command at your prompt:")
		fmt.Println()
		fmt.Printf("     set -Ux PAGER %s\n", getMoorPath())
	} else if shellIsPowershell {
		fmt.Println("  Put the following line in your $PROFILE file (\"echo $PROFILE\" to find it)")
		fmt.Println("  and moor will be used as the default pager in all new terminal windows:")
		fmt.Println()
		fmt.Printf("     $env:PAGER = \"%s\"\n", getMoorPath())
	} else {
		// I don't know how to identify bash / zsh, put generic instructions here
		fmt.Println("  Put the following line in your ~/.bashrc, ~/.bash_profile or ~/.zshrc")
		fmt.Println("  and moor will be used as the default pager in all new terminal windows:")
		fmt.Println()
		fmt.Printf("     export PAGER=%s\n", getMoorPath())
	}
}

// "moor" if we're in the $PATH, otherwise an absolute path
func getMoorPath() string {
	moorPath := os.Args[0]
	if filepath.IsAbs(moorPath) {
		return moorPath
	}

	if strings.Contains(moorPath, string(os.PathSeparator)) {
		// Relative path
		moorPath, err := filepath.Abs(moorPath)
		if err != nil {
			panic(err)
		}
		return moorPath
	}

	// Neither absolute nor relative, try PATH
	_, err := exec.LookPath(moorPath)
	if err != nil {
		panic("Unable to find in $PATH: " + moorPath)
	}
	return moorPath
}

func absLookPath(path string) (string, error) {
	lookedPath, err := exec.LookPath(path)
	if err != nil {
		return "", err
	}

	absLookedPath, err := filepath.Abs(lookedPath)
	if err != nil {
		return "", err
	}

	return absLookedPath, err
}
