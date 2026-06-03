package internal

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// ParseWarning represents a non-fatal issue found while parsing a keybindings
// file. The structured fields let callers act on the warning without parsing
// the human-readable message string.
type ParseWarning struct {
	Line    int
	Section string // section name like "viewing", or "" if outside any section
	Message string
}

// Error implements the error interface so ParseWarning can be used wherever an
// error is expected, and so that existing code calling .Error() keeps working.
func (w ParseWarning) Error() string {
	return fmt.Sprintf("line %d: %s", w.Line, w.Message)
}

// ParseKeybindingsFile parses a keybindings file and returns the resulting
// bindings.  The file is overlaid on top of the defaults: every key the user
// lists overrides the default for that key, and all other defaults are kept.
// The special directive !clear (anywhere in the file) resets every section to
// empty before processing continues, giving a clean-slate starting point.
func ParseKeybindingsFile(path string) (ModeBindings, []ParseWarning, error) {
	file, err := os.Open(path)
	if err != nil {
		return ModeBindings{}, nil, err
	}
	defer func() { _ = file.Close() }()

	// Ensure reverse maps are built
	reverseMapOnce.Do(buildReverseMaps)

	result := DefaultModeBindings()
	var warnings []ParseWarning
	var currentSection string

	// buildSectionBindings returns a fresh map pointing into the current result.
	// It must be called whenever result is replaced (e.g. on !clear).
	buildSectionBindings := func() map[string]*KeyBindings[Action] {
		m := make(map[string]*KeyBindings[Action], len(actionSections))
		for _, s := range actionSections {
			m[s.name] = s.field(&result)
		}
		return m
	}
	sectionBindings := buildSectionBindings()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and blank lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sectionName := strings.TrimSpace(line[1 : len(line)-1])
			currentSection = sectionName
			_, isActionSection := sectionBindings[sectionName]
			if !isActionSection && sectionName != "input" {
				warnings = append(warnings, ParseWarning{Line: lineNum, Section: "", Message: fmt.Sprintf("unknown section [%s]", sectionName)})
			}
			continue
		}

		// !clear resets every section to empty; typically at the top of the file.
		if line == "!clear" {
			result = EmptyModeBindings()
			sectionBindings = buildSectionBindings()
			continue
		}

		if currentSection == "" {
			warnings = append(warnings, ParseWarning{Line: lineNum, Section: "", Message: "binding outside of section"})
			continue
		}

		// Split into key and action
		fields := strings.Fields(line)
		if len(fields) != 2 {
			warnings = append(warnings, ParseWarning{Line: lineNum, Section: currentSection, Message: fmt.Sprintf("expected 2 fields (key action), got %d", len(fields))})
			continue
		}

		keyName := fields[0]
		actionName := strings.ToLower(fields[1])

		// Parse the key
		keyCode, isKeyCode := parseKeyCodeName(keyName)
		r, isRune := parseRuneKeyName(keyName)

		if !isKeyCode && !isRune {
			warnings = append(warnings, ParseWarning{Line: lineNum, Section: currentSection, Message: fmt.Sprintf("unknown key name %q", keyName)})
			continue
		}

		if kb, ok := sectionBindings[currentSection]; ok {
			action, ok := actionNamesReverse[actionName]
			if !ok {
				warnings = append(warnings, ParseWarning{Line: lineNum, Section: currentSection, Message: fmt.Sprintf("unknown action %q", actionName)})
				continue
			}
			if isKeyCode {
				kb.KeyCodeBindings[keyCode] = action
			} else {
				kb.RuneBindings[r] = action
			}
		} else if currentSection == "input" {
			action, ok := inputActionNamesReverse[actionName]
			if !ok {
				warnings = append(warnings, ParseWarning{Line: lineNum, Section: currentSection, Message: fmt.Sprintf("unknown action %q", actionName)})
				continue
			}
			if isKeyCode {
				result.Input.KeyCodeBindings[keyCode] = action
			} else {
				result.Input.RuneBindings[r] = action
			}
		}
		// unknown sections: silently skip (already warned about the section header)
	}

	if err := scanner.Err(); err != nil {
		return ModeBindings{}, warnings, err
	}

	return result, warnings, nil
}
