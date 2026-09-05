package internal

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/reader"
)

// Dump the reader lines into a read-only temp file and return the absolute file
// name.
func dumpToTempFile(reader *reader.ReaderImpl) (string, error) {
	tempFile, err := os.CreateTemp("", "moor-contents-")
	if err != nil {
		return "", err
	}
	defer func() {
		err = tempFile.Close()
		if err != nil {
			log.Warn("Failed to close temp file: ", err)
		}
	}()

	log.Debug("Dumping contents into: ", tempFile.Name())

	lines := reader.GetLines(linemetadata.Index{}, math.MaxInt)
	for _, line := range lines.Lines {
		toWrite := line.Plain()
		_, err := tempFile.WriteString(toWrite + "\n")
		if err != nil {
			return "", err
		}
	}

	// Ref: https://pkg.go.dev/os#Chmod
	err = os.Chmod(tempFile.Name(), 0400)
	if err != nil {
		// Doesn't matter that much, but if it fails we should at least log it
		log.Debug("Failed to make temp file ", tempFile.Name(), " read-only: ", err)
	}

	return tempFile.Name(), nil
}

// Check that the editor is executable
func errUnlessExecutable(file string) error {
	stat, err := os.Stat(file)
	if err != nil {
		return fmt.Errorf("Failed to stat %s: %w", file, err)
	}

	if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(file), ".exe") {
		log.Debug(".exe file on Windows, assuming executable: ", file)
		return nil
	}

	if stat.Mode()&0111 != 0 {
		// Note that this check isn't perfect, it could still be executable but
		// not by us. Corner case, let's just fail later in that case.
		return nil
	}

	return fmt.Errorf("Not executable: %s", file)
}

// pickAnEditor returns the editor command to execute, where it came from
// ("VISUAL", "EDITOR" or "fallback list"), and an error if no editor could
// be found.
func pickAnEditor() (string, string, error) {
	// Get an editor setting from either VISUAL or EDITOR
	editorEnv := "VISUAL"
	editor := strings.TrimSpace(os.Getenv(editorEnv))
	if editor == "" {
		editorEnv := "EDITOR"
		editor = strings.TrimSpace(os.Getenv(editorEnv))
	}

	if editor != "" {
		return editor, editorEnv, nil
	}

	candidates := []string{
		"vim", // This is a sucky default, but let's have it for compatibility with less
		"nano",
		"vi",
	}

	for _, candidate := range candidates {
		fullPath, err := exec.LookPath(candidate)
		log.Trace("Problem finding ", candidate, ": ", err)
		if err != nil {
			continue
		}

		err = errUnlessExecutable(fullPath)
		log.Trace("Problem with executability of ", fullPath, ": ", err)
		if err != nil {
			continue
		}

		return candidate, "fallback list", nil
	}

	return "", "", fmt.Errorf("No editor found, tried: $VISUAL, $EDITOR, %s", strings.Join(candidates, ", "))
}

// Returns nil if nothing is being shown.
func middleVisibleLine(p *Pager) *linemetadata.Index {
	lines := p.renderLines().lines
	if len(lines) == 0 {
		return nil
	}

	return ptr.To(lines[len(lines)/2].inputLineIndex)
}

// check if the editor can accept "+LINENUMBER" argument
func editorAcceptsPlusLine(editorCommand string) bool {
	base := strings.ToLower(filepath.Base(editorCommand))
	base = strings.TrimSuffix(base, ".exe")

	// hardcode known editors for now
	switch base {
	case "vim", "vi", "nvim", "nano", "pico", "emacs":
		return true
	}
	return false
}

func handleEditingRequest(p *Pager) {
	if os.Getenv("LESSSECURE") == "1" {
		p.mode = &PagerModeInfo{
			Pager: p,
			Text:  "Not launching editor since LESSSECURE=1 is set in the environment",
		}
		return
	}

	editor, editorEnv, err := pickAnEditor()
	if err != nil {
		log.Warn("Failed to find an editor: ", err)
		return
	}

	// Tyre kicking check that we can find the editor either in the PATH or as
	// an absolute path
	firstWord := strings.Fields(editor)[0]
	editorPath, err := exec.LookPath(firstWord)
	if err != nil {
		// FIXME: Show a message in the status bar instead? Nothing wrong with
		// moor here.
		log.Warn("Failed to find editor "+firstWord+" from $"+editorEnv+": ", err)
		return
	}

	// Check that the editor is executable
	err = errUnlessExecutable(editorPath)
	if err != nil {
		// FIXME: Show a message in the status bar instead? Nothing wrong with
		// moor here.
		log.Warn("Editor from {} not executable: {}", editorEnv, err)
		return
	}

	canOpenFile := p.readers[p.currentReader].FileName != nil
	if p.readers[p.currentReader].FileName != nil {
		// Verify that the file exists and is readable
		err = reader.TryOpen(*p.readers[p.currentReader].FileName)
		if err != nil {
			canOpenFile = false
			log.Info("File to edit is not readable: ", err)
		}
	}

	var fileToEdit string
	if canOpenFile {
		fileToEdit = *p.readers[p.currentReader].FileName
	} else {
		// NOTE: Let's not wait for the stream to finish, just dump whatever we
		// have and open the editor on that. The user just asked for it, if they
		// wanted to wait, they should have done that themselves.

		// Create a temp file based on reader contents
		fileToEdit, err = dumpToTempFile(p.readers[p.currentReader])
		if err != nil {
			log.Warn("Failed to create temp file to edit: ", err)
			return
		}

		// Clean up the temp file
		defer func() {
			err = os.Remove(fileToEdit)
			if err != nil {
				log.Warn("Failed to remove temp file ", fileToEdit, ": ", err)
			} else {
				log.Debug("Removed temp file: ", fileToEdit)
			}
		}()
	}

	err = p.screen.PauseAndCall(func() error {
		// NOTE: If you do any changes here, make sure they work with both "nano"
		// and "code -w" (VSCode).
		commandWithArgs := strings.Fields(editor)

		lineInfo := ""
		// add line number argument if editor is supported
		//
		// vim / nano / etc all center the requested line on screen, so aim for
		// the middle of moor's screen to match what the user was actually
		// looking at.
		if middleLine := middleVisibleLine(p); middleLine != nil && editorAcceptsPlusLine(commandWithArgs[0]) {
			lineNumber := middleLine.Index() + 1
			lineInfo = fmt.Sprintf(" (at line %d)", lineNumber)
			commandWithArgs = append(commandWithArgs, fmt.Sprintf("+%d", lineNumber))
		}

		commandWithArgs = append(commandWithArgs, fileToEdit)

		log.Info("'v' pressed, launching editor", lineInfo, ": ", commandWithArgs)
		command := exec.Command(commandWithArgs[0], commandWithArgs[1:]...)

		if runtime.GOOS == "windows" {
			// Don't touch command.Stdin on Windows:
			// https://github.com/walles/moor/issues/281#issuecomment-2953384726
		} else {
			// Since os.Stdin might come from a pipe, we can't trust that. Instead,
			// we tell the editor to read from os.Stdout, which points to the
			// terminal as well.
			//
			// Tested on macOS and Linux, works like a charm.
			command.Stdin = os.Stdout // <- YES, WE SHOULD ASSIGN STDOUT TO STDIN
		}

		command.Stdout = os.Stdout
		command.Stderr = os.Stderr

		err := command.Run()
		if err == nil {
			log.Info("Editor exited successfully: ", commandWithArgs)
		}
		return err
	})
	if err != nil {
		log.Warn("Failed to launch editor in paused session: ", err)
		p.mode = &PagerModeInfo{
			Pager: p,
			Text:  "Failed to launch editor \"" + editor + "\": " + err.Error(),
		}

		return
	}
}
