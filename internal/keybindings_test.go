package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/walles/moor/v2/twin"
)

// TestParseKeyCodeName tests parsing of special key names
func TestParseKeyCodeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected twin.KeyCode
		valid    bool
	}{
		// Standard special keys
		{"escape", "escape", twin.KeyEscape, true},
		{"enter", "enter", twin.KeyEnter, true},
		{"backspace", "backspace", twin.KeyBackspace, true},
		{"delete", "delete", twin.KeyDelete, true},
		{"up", "up", twin.KeyUp, true},
		{"down", "down", twin.KeyDown, true},
		{"left", "left", twin.KeyLeft, true},
		{"right", "right", twin.KeyRight, true},
		{"home", "home", twin.KeyHome, true},
		{"end", "end", twin.KeyEnd, true},
		{"pageup", "pageup", twin.KeyPgUp, true},
		{"pagedown", "pagedown", twin.KeyPgDown, true},
		{"pgup", "pgup", twin.KeyPgUp, true},
		{"pgdown", "pgdown", twin.KeyPgDown, true},

		// Alt-modified keys
		{"alt-up", "alt-up", twin.KeyAltUp, true},
		{"alt-down", "alt-down", twin.KeyAltDown, true},
		{"alt-left", "alt-left", twin.KeyAltLeft, true},
		{"alt-right", "alt-right", twin.KeyAltRight, true},

		// Case insensitivity
		{"ESCAPE", "ESCAPE", twin.KeyEscape, true},
		{"EsCaPe", "EsCaPe", twin.KeyEscape, true},
		{"ALT-UP", "ALT-UP", twin.KeyAltUp, true},
		{"Alt-Up", "Alt-Up", twin.KeyAltUp, true},
		{"PaGeDoWn", "PaGeDoWn", twin.KeyPgDown, true},

		// Invalid inputs
		{"unknown-key", "unknown-key", 0, false},
		{"alt-x", "alt-x", 0, false},
		{"alt-", "alt-", 0, false},
		{"ctrl-a", "ctrl-a", 0, false},
		{"", "", 0, false},
		{"space", "space", 0, false}, // space is a rune, not a keycode
		{"a", "a", 0, false},         // single char is a rune
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := parseKeyCodeName(tt.input)
			if valid != tt.valid {
				t.Errorf("parseKeyCodeName(%q) valid = %v, want %v", tt.input, valid, tt.valid)
			}
			if valid && got != tt.expected {
				t.Errorf("parseKeyCodeName(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

// TestParseRuneKeyName tests parsing of rune key names
func TestParseRuneKeyName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected rune
		valid    bool
	}{
		// Control characters
		{"ctrl-a", "ctrl-a", 1, true},
		{"ctrl-b", "ctrl-b", 2, true},
		{"ctrl-c", "ctrl-c", 3, true},
		{"ctrl-d", "ctrl-d", 4, true},
		{"ctrl-n", "ctrl-n", 14, true},
		{"ctrl-p", "ctrl-p", 16, true},
		{"ctrl-t", "ctrl-t", 20, true},
		{"ctrl-u", "ctrl-u", 21, true},
		{"ctrl-z", "ctrl-z", 26, true},

		// Special rune names
		{"space", "space", ' ', true},
		{"space-upper", "SPACE", ' ', true},
		{"space-mixed", "Space", ' ', true},
		{"tab", "tab", '\t', true},
		{"tab-escape", "\\t", '\t', true},
		{"newline", "\\n", '\n', true},

		// Single characters
		{"letter-a", "a", 'a', true},
		{"letter-z", "z", 'z', true},
		{"letter-A", "A", 'A', true},
		{"letter-Z", "Z", 'Z', true},
		{"digit-0", "0", '0', true},
		{"digit-9", "9", '9', true},
		{"slash", "/", '/', true},
		{"question", "?", '?', true},
		{"ampersand", "&", '&', true},
		{"colon", ":", ':', true},
		{"equals", "=", '=', true},
		{"less-than", "<", '<', true},
		{"greater-than", ">", '>', true},
		{"single-quote", "'", '\'', true},

		// Invalid inputs
		{"CTRL-A", "CTRL-A", 0, false}, // uppercase not supported
		{"Ctrl-A", "Ctrl-A", 0, false}, // mixed case not supported
		{"ctrl-A", "ctrl-A", 0, false}, // uppercase letter not supported
		{"ctrl-invalid", "^1", 0, false},
		{"ctrl-invalid-2", "^@", 0, false},
		{"multi-char", "ab", 0, false},
		{"empty", "", 0, false},
		{"escape", "escape", 0, false}, // escape is a keycode
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := parseRuneKeyName(tt.input)
			if valid != tt.valid {
				t.Errorf("parseRuneKeyName(%q) valid = %v, want %v", tt.input, valid, tt.valid)
			}
			if valid && got != tt.expected {
				t.Errorf("parseRuneKeyName(%q) = %d (%c), want %d (%c)", tt.input, got, got, tt.expected, tt.expected)
			}
		})
	}
}

// TestActionNameReverseMaps verifies that every action enum has a name entry and round-trips correctly
func TestActionNameReverseMaps(t *testing.T) {
	// Build reverse maps
	reverseMapOnce.Do(buildReverseMaps)

	t.Run("ViewingActions", func(t *testing.T) {
		// Check all viewing actions have names
		for action := NoAction; action <= ScrollHome; action++ {
			name, ok := actionNames[action]
			if !ok {
				t.Errorf("Action %d has no name", action)
				continue
			}

			// Check reverse lookup
			reversedAction, ok := actionNamesReverse[name]
			if !ok {
				t.Errorf("Action name %q not in reverse map", name)
				continue
			}

			if reversedAction != action {
				t.Errorf("Action %d -> %q -> %d (mismatch)", action, name, reversedAction)
			}
		}

		// Check no duplicate names
		seen := make(map[string]Action)
		for action, name := range actionNames {
			if prev, exists := seen[name]; exists {
				t.Errorf("Duplicate name %q for actions %d and %d", name, prev, action)
			}
			seen[name] = action
		}
	})

	t.Run("SearchActions", func(t *testing.T) {
		for action := NoAction; action <= HistoryNext; action++ {
			name, ok := actionNames[action]
			if !ok {
				t.Errorf("Action %d has no name", action)
				continue
			}

			reversedAction, ok := actionNamesReverse[name]
			if !ok {
				t.Errorf("Action name %q not in reverse map", name)
				continue
			}

			if reversedAction != action {
				t.Errorf("Action %d -> %q -> %d (mismatch)", action, name, reversedAction)
			}
		}
	})

	t.Run("FilterActions", func(t *testing.T) {
		for action := NoAction; action <= ScrollPageDown; action++ {
			name, ok := actionNames[action]
			if !ok {
				t.Errorf("Action %d has no name", action)
				continue
			}

			reversedAction, ok := actionNamesReverse[name]
			if !ok {
				t.Errorf("Action name %q not in reverse map", name)
				continue
			}

			if reversedAction != action {
				t.Errorf("Action %d -> %q -> %d (mismatch)", action, name, reversedAction)
			}
		}
	})

	t.Run("GotoLineActions", func(t *testing.T) {
		for action := NoAction; action <= Quit; action++ {
			name, ok := actionNames[action]
			if !ok {
				t.Errorf("Action %d has no name", action)
				continue
			}

			reversedAction, ok := actionNamesReverse[name]
			if !ok {
				t.Errorf("Action name %q not in reverse map", name)
				continue
			}

			if reversedAction != action {
				t.Errorf("Action %d -> %q -> %d (mismatch)", action, name, reversedAction)
			}
		}
	})

	t.Run("ColonCommandActions", func(t *testing.T) {
		for action := NoAction; action <= Quit; action++ {
			name, ok := actionNames[action]
			if !ok {
				t.Errorf("Action %d has no name", action)
				continue
			}

			reversedAction, ok := actionNamesReverse[name]
			if !ok {
				t.Errorf("Action name %q not in reverse map", name)
				continue
			}

			if reversedAction != action {
				t.Errorf("Action %d -> %q -> %d (mismatch)", action, name, reversedAction)
			}
		}
	})
}

// TestBuildKeyBindings verifies that building key bindings from default slices doesn't panic
func TestBuildKeyBindings(t *testing.T) {
	t.Run("ViewingBindings", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("buildKeyBindings panicked: %v", r)
			}
		}()
		kb := buildKeyBindings(defaultViewingEntries)
		if kb.KeyCodeBindings == nil {
			t.Error("KeyCodeBindings is nil")
		}
		if kb.RuneBindings == nil {
			t.Error("RuneBindings is nil")
		}
		if len(kb.KeyCodeBindings) == 0 && len(kb.RuneBindings) == 0 {
			t.Error("No bindings were created")
		}
	})

	t.Run("SearchBindings", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("buildKeyBindings panicked: %v", r)
			}
		}()
		kb := buildKeyBindings(defaultSearchEntries)
		if kb.KeyCodeBindings == nil || kb.RuneBindings == nil {
			t.Error("Bindings maps are nil")
		}
	})

	t.Run("FilterBindings", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("buildKeyBindings panicked: %v", r)
			}
		}()
		kb := buildKeyBindings(defaultFilterEntries)
		if kb.KeyCodeBindings == nil || kb.RuneBindings == nil {
			t.Error("Bindings maps are nil")
		}
	})

	t.Run("GotoLineBindings", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("buildKeyBindings panicked: %v", r)
			}
		}()
		kb := buildKeyBindings(defaultGotoLineEntries)
		if kb.KeyCodeBindings == nil || kb.RuneBindings == nil {
			t.Error("Bindings maps are nil")
		}
	})

	t.Run("ColonCommandBindings", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("buildKeyBindings panicked: %v", r)
			}
		}()
		kb := buildKeyBindings(defaultColonCommandEntries)
		if kb.KeyCodeBindings == nil || kb.RuneBindings == nil {
			t.Error("Bindings maps are nil")
		}
	})
}

// TestDefaultKeybindingsTextRoundTrip verifies that the default keybindings text can be parsed back
func TestDefaultKeybindingsTextRoundTrip(t *testing.T) {
	text := DefaultKeybindingsText()
	if text == "" {
		t.Fatal("DefaultKeybindingsText returned empty string")
	}

	// Verify all expected sections are present
	expectedSections := []string{"[viewing]", "[search]", "[filter]", "[goto-line]", "[colon-command]", "[input]"}
	for _, section := range expectedSections {
		if !strings.Contains(text, section) {
			t.Errorf("DefaultKeybindingsText missing section %s", section)
		}
	}

	// Write to a temp file and parse it back
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")

	err := os.WriteFile(tmpFile, []byte(text), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to parse default keybindings text: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got %d warnings parsing default keybindings text:", len(warnings))
		for _, w := range warnings {
			t.Errorf("  %v", w)
		}
	}

	// Verify key bindings are present
	if _, ok := bindings.Viewing.RuneBindings['q']; !ok {
		t.Error("'q' binding missing from viewing mode")
	}
	if _, ok := bindings.Viewing.KeyCodeBindings[twin.KeyEscape]; !ok {
		t.Error("'escape' binding missing from viewing mode")
	}
}

// TestParserCommentsAndBlankLines verifies that comments and blank lines are ignored
func TestParserCommentsAndBlankLines(t *testing.T) {
	input := `
# This is a comment
[viewing]
# Another comment
q  quit

  # Indented comment
r  reload
# Comment at end
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	if action, ok := bindings.Viewing.RuneBindings['q']; !ok || action != Quit {
		t.Error("'q' binding not parsed correctly")
	}

	if action, ok := bindings.Viewing.RuneBindings['r']; !ok || action != Reload {
		t.Error("'r' binding not parsed correctly")
	}
}

// TestParserUnknownSection verifies that unknown sections generate warnings
func TestParserUnknownSection(t *testing.T) {
	input := `
[unknown-section]
a  some-action

[viewing]
q  quit
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) == 0 {
		t.Error("Expected warning for unknown section")
	}

	foundUnknownSectionWarning := false
	for _, w := range warnings {
		if strings.Contains(w.Error(), "unknown section") {
			foundUnknownSectionWarning = true
			break
		}
	}

	if !foundUnknownSectionWarning {
		t.Errorf("Expected 'unknown section' warning, got: %v", warnings)
	}

	// The valid [viewing] section must still have been parsed
	if action, ok := bindings.Viewing.RuneBindings['q']; !ok || action != Quit {
		t.Error("Valid viewing section was not parsed correctly")
	}
}

// TestParserBadFieldCount verifies that lines with wrong field count generate warnings
func TestParserBadFieldCount(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"one-field", "[viewing]\nq\n"},
		{"three-fields", "[viewing]\nq quit extra\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "keybindings")
			err := os.WriteFile(tmpFile, []byte(tt.input), 0644)
			if err != nil {
				t.Fatalf("Failed to write temp file: %v", err)
			}

			_, warnings, err := ParseKeybindingsFile(tmpFile)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			if len(warnings) == 0 {
				t.Error("Expected warning for bad field count")
			}

			foundFieldCountWarning := false
			for _, w := range warnings {
				if strings.Contains(w.Error(), "expected 2 fields") {
					foundFieldCountWarning = true
					break
				}
			}

			if !foundFieldCountWarning {
				t.Errorf("Expected 'expected 2 fields' warning, got: %v", warnings)
			}
		})
	}
}

// TestParserUnknownKey verifies that unknown keys generate warnings
func TestParserUnknownKey(t *testing.T) {
	input := `
[viewing]
unknown-key  quit
ctrl-1  quit
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) < 2 {
		t.Errorf("Expected at least 2 warnings for unknown keys, got %d", len(warnings))
	}

	unknownKeyCount := 0
	for _, w := range warnings {
		if strings.Contains(w.Error(), "unknown key name") {
			unknownKeyCount++
		}
	}

	if unknownKeyCount < 2 {
		t.Errorf("Expected 2 'unknown key name' warnings, got %d", unknownKeyCount)
	}
}

// TestParserUnknownAction verifies that unknown actions generate warnings
func TestParserUnknownAction(t *testing.T) {
	input := `
[viewing]
q  unknown-action
r  another-bad-action

[search]
enter  bad-search-action
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) < 3 {
		t.Errorf("Expected at least 3 warnings for unknown actions, got %d", len(warnings))
	}

	unknownActionCount := 0
	for _, w := range warnings {
		if strings.Contains(w.Error(), "unknown action") {
			unknownActionCount++
		}
	}

	if unknownActionCount < 3 {
		t.Errorf("Expected 3 'unknown action' warnings, got %d", unknownActionCount)
	}
}

// TestParserNoAction verifies that 'noaction' maps to zero values
func TestParserNoAction(t *testing.T) {
	input := `
[viewing]
q  noaction

[search]
enter  noaction

[filter]
escape  noaction

[goto-line]
g  noaction

[colon-command]
n  noaction
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	if action, ok := bindings.Viewing.RuneBindings['q']; !ok || action != NoAction {
		t.Errorf("'q' should map to NoAction (0), got %d", action)
	}

	if action, ok := bindings.Search.KeyCodeBindings[twin.KeyEnter]; !ok || action != NoAction {
		t.Errorf("'enter' should map to NoAction (0), got %d", action)
	}

	if action, ok := bindings.Filter.KeyCodeBindings[twin.KeyEscape]; !ok || action != NoAction {
		t.Errorf("'escape' should map to NoAction (0), got %d", action)
	}

	if action, ok := bindings.GotoLine.RuneBindings['g']; !ok || action != NoAction {
		t.Errorf("'g' should map to NoAction (0), got %d", action)
	}

	if action, ok := bindings.ColonCommand.RuneBindings['n']; !ok || action != NoAction {
		t.Errorf("'n' should map to NoAction (0), got %d", action)
	}
}

// TestParserVariousKeyFormats verifies various key formats work correctly
func TestParserVariousKeyFormats(t *testing.T) {
	input := `
[viewing]
# Special keys
escape  quit
enter  scroll-down
pageup  scroll-page-up
pagedown  scroll-page-down
up  scroll-up
down  scroll-down

# Alt keys
alt-up  scroll-up
alt-down  scroll-down
alt-left  scroll-left
alt-right  scroll-right

# Control characters
ctrl-a  scroll-home
ctrl-d  scroll-half-page-down
ctrl-u  scroll-half-page-up

# Single characters
q  quit
/  search-forward
?  search-backward
space  scroll-page-down

# Case-sensitive single chars
G  scroll-bottom
N  previous-search-hit
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	kb := bindings.Viewing

	// Check special keys
	tests := []struct {
		keyCode twin.KeyCode
		action  Action
	}{
		{twin.KeyEscape, Quit},
		{twin.KeyEnter, ScrollDown},
		{twin.KeyPgUp, ScrollPageUp},
		{twin.KeyPgDown, ScrollPageDown},
		{twin.KeyUp, ScrollUp},
		{twin.KeyDown, ScrollDown},
		{twin.KeyAltUp, ScrollUp},
		{twin.KeyAltDown, ScrollDown},
		{twin.KeyAltLeft, ScrollLeft},
		{twin.KeyAltRight, ScrollRight},
	}

	for _, tt := range tests {
		if action, ok := kb.KeyCodeBindings[tt.keyCode]; !ok || action != tt.action {
			t.Errorf("KeyCode %d should map to action %d, got %d (ok=%v)", tt.keyCode, tt.action, action, ok)
		}
	}

	// Check rune bindings
	runeTests := []struct {
		r      rune
		action Action
	}{
		{1, ScrollHome},         // ^A
		{4, ScrollHalfPageDown}, // ^D
		{21, ScrollHalfPageUp},  // ^U
		{'q', Quit},
		{'/', SearchForward},
		{'?', SearchBackward},
		{' ', ScrollPageDown},
		{'G', ScrollBottom},
		{'N', PreviousSearchHit},
	}

	for _, tt := range runeTests {
		if action, ok := kb.RuneBindings[tt.r]; !ok || action != tt.action {
			t.Errorf("Rune %d (%c) should map to action %d, got %d (ok=%v)", tt.r, tt.r, tt.action, action, ok)
		}
	}
}

// TestParserPerKeyMerge verifies that the parser overlays user-specified keys
// on top of defaults: overridden keys change, everything else is preserved.
func TestParserPerKeyMerge(t *testing.T) {
	input := `
[viewing]
escape  help
q  reload
x  quit
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	// Overridden / new keys
	if action, ok := bindings.Viewing.KeyCodeBindings[twin.KeyEscape]; !ok || action != Help {
		t.Errorf("Viewing escape should be Help, got %d", action)
	}
	if action, ok := bindings.Viewing.RuneBindings['q']; !ok || action != Reload {
		t.Errorf("Viewing 'q' should be Reload, got %d", action)
	}
	if action, ok := bindings.Viewing.RuneBindings['x']; !ok || action != Quit {
		t.Errorf("Viewing 'x' should be Quit, got %d", action)
	}

	// All unmentioned viewing defaults must be intact.
	defaults := DefaultModeBindings()
	for k, defaultAction := range defaults.Viewing.RuneBindings {
		if k == 'q' || k == 'x' {
			continue
		}
		if got, ok := bindings.Viewing.RuneBindings[k]; !ok || got != defaultAction {
			t.Errorf("Viewing rune %c: expected default %d, got %d (ok=%v)", k, defaultAction, got, ok)
		}
	}
	for k, defaultAction := range defaults.Viewing.KeyCodeBindings {
		if k == twin.KeyEscape {
			continue
		}
		if got, ok := bindings.Viewing.KeyCodeBindings[k]; !ok || got != defaultAction {
			t.Errorf("Viewing keycode %d: expected default %d, got %d (ok=%v)", k, defaultAction, got, ok)
		}
	}

	// Untouched sections must equal their defaults.
	if action, ok := bindings.Search.KeyCodeBindings[twin.KeyEnter]; !ok || action != defaults.Search.KeyCodeBindings[twin.KeyEnter] {
		t.Error("Search Enter binding doesn't match default")
	}
	if action, ok := bindings.GotoLine.RuneBindings['g']; !ok || action != defaults.GotoLine.RuneBindings['g'] {
		t.Error("GotoLine 'g' binding doesn't match default")
	}
}

// TestParserClear verifies that !clear resets all defaults; only user-specified
// keys survive, and sections not mentioned at all end up empty.
func TestParserClear(t *testing.T) {
	input := `
!clear

[viewing]
x  quit

[search]
enter  accept
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	// Explicitly listed keys must be present.
	if action, ok := bindings.Viewing.RuneBindings['x']; !ok || action != Quit {
		t.Error("Viewing 'x' should be Quit")
	}
	if action, ok := bindings.Search.KeyCodeBindings[twin.KeyEnter]; !ok || action != Accept {
		t.Error("Search Enter should be Accept")
	}

	// All defaults must be gone from viewing.
	defaults := DefaultModeBindings()
	for k := range defaults.Viewing.RuneBindings {
		if _, ok := bindings.Viewing.RuneBindings[k]; ok {
			t.Errorf("Viewing default rune %c survived !clear", k)
		}
	}
	for k := range defaults.Viewing.KeyCodeBindings {
		if _, ok := bindings.Viewing.KeyCodeBindings[k]; ok {
			t.Errorf("Viewing default keycode %d survived !clear", k)
		}
	}

	// Unmentioned defaults must be gone from search too.
	for k := range defaults.Search.KeyCodeBindings {
		if k == twin.KeyEnter {
			continue // user listed this
		}
		if _, ok := bindings.Search.KeyCodeBindings[k]; ok {
			t.Errorf("Search default keycode %d survived !clear", k)
		}
	}

	// GotoLine was not mentioned at all — must be completely empty.
	if len(bindings.GotoLine.KeyCodeBindings) != 0 || len(bindings.GotoLine.RuneBindings) != 0 {
		t.Error("GotoLine should be empty when !clear is set and section is not mentioned")
	}
}

// TestNoActionUnbindsKey verifies that 'noaction' unbinds a specific key
// without disturbing any other defaults.
func TestNoActionUnbindsKey(t *testing.T) {
	input := `
[viewing]
q  noaction
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	// 'q' must be unbound.
	if action, ok := bindings.Viewing.RuneBindings['q']; !ok || action != NoAction {
		t.Errorf("Expected 'q' -> NoAction, got action=%d ok=%v", action, ok)
	}

	// All other viewing defaults must be intact.
	defaults := DefaultModeBindings()
	for k, defaultAction := range defaults.Viewing.RuneBindings {
		if k == 'q' {
			continue
		}
		if got, ok := bindings.Viewing.RuneBindings[k]; !ok || got != defaultAction {
			t.Errorf("Viewing rune %c: expected default %d, got %d (ok=%v)", k, defaultAction, got, ok)
		}
	}
}

// TestParserBindingOutsideSection verifies warning for bindings outside a section
func TestParserBindingOutsideSection(t *testing.T) {
	input := `
# Comment
q  quit

[viewing]
r  reload
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	_, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) == 0 {
		t.Error("Expected warning for binding outside section")
	}

	foundWarning := false
	for _, w := range warnings {
		if strings.Contains(w.Error(), "binding outside of section") {
			foundWarning = true
			break
		}
	}

	if !foundWarning {
		t.Errorf("Expected 'binding outside of section' warning, got: %v", warnings)
	}
}

// TestDefaultModeBindingsIdempotent verifies DefaultModeBindings returns equivalent data on every call
func TestDefaultModeBindingsIdempotent(t *testing.T) {
	first := DefaultModeBindings()
	second := DefaultModeBindings()

	// Each call returns independent copies (maps are cloned), but the data must be identical.
	if len(first.Viewing.KeyCodeBindings) != len(second.Viewing.KeyCodeBindings) {
		t.Error("Viewing KeyCodeBindings have different lengths")
	}

	// Verify some specific bindings match
	if first.Viewing.RuneBindings['q'] != second.Viewing.RuneBindings['q'] {
		t.Error("Viewing RuneBindings for 'q' don't match")
	}

	if first.Search.KeyCodeBindings[twin.KeyEnter] != second.Search.KeyCodeBindings[twin.KeyEnter] {
		t.Error("Search KeyCodeBindings for Enter don't match")
	}
}

// TestDefaultModeBindingsMutationIsolation verifies that mutating a returned
// ModeBindings does not corrupt the shared cache or subsequent calls.
func TestDefaultModeBindingsMutationIsolation(t *testing.T) {
	first := DefaultModeBindings()

	// Remember the original action for 'q' in Viewing mode.
	originalAction := first.Viewing.RuneBindings['q']

	// Mutate the returned copy in place.
	first.Viewing.RuneBindings['q'] = NoAction

	// A fresh call must return the unmodified (cached) data.
	second := DefaultModeBindings()
	if second.Viewing.RuneBindings['q'] != originalAction {
		t.Errorf("cache was corrupted: expected 'q' -> %v, got %v", originalAction, second.Viewing.RuneBindings['q'])
	}
}

// TestParserMultipleSectionsSameMode verifies that later bindings override earlier ones
func TestParserMultipleSectionsSameMode(t *testing.T) {
	input := `
[viewing]
q  quit

[viewing]
q  reload
r  quit
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	// Later binding should override
	if action, ok := bindings.Viewing.RuneBindings['q']; !ok || action != Reload {
		t.Errorf("'q' should be Reload (overridden), got %d", action)
	}

	if action, ok := bindings.Viewing.RuneBindings['r']; !ok || action != Quit {
		t.Error("'r' binding from second section not found")
	}
}

// TestInputActionRoundTrip tests that input action names round-trip correctly
func TestInputActionRoundTrip(t *testing.T) {
	// Ensure reverse maps are built
	reverseMapOnce.Do(buildReverseMaps)

	// Check all input actions have names
	expectedActions := []InputAction{
		NoInputAction,
		CursorLeft,
		CursorRight,
		CursorHome,
		CursorEnd,
		Backspace,
		Delete,
		DeleteToEnd,
		DeleteToStart,
	}

	for _, action := range expectedActions {
		name, ok := inputActionNames[action]
		if !ok {
			t.Errorf("InputAction %d missing name in inputActionNames", action)
			continue
		}

		reversedAction, ok := inputActionNamesReverse[name]
		if !ok {
			t.Errorf("InputAction name %q missing in inputActionNamesReverse", name)
			continue
		}

		if reversedAction != action {
			t.Errorf("InputAction %d -> name %q -> action %d (mismatch)", action, name, reversedAction)
		}
	}
}

// TestInputBindingsParser tests parsing of [input] section
func TestInputBindingsParser(t *testing.T) {
	input := `
[input]
left          cursor-left
right         cursor-right
home          cursor-home
end           cursor-end
backspace     backspace
delete        delete
ctrl-a        cursor-home
ctrl-e        cursor-end
ctrl-b        cursor-left
ctrl-f        cursor-right
ctrl-k        delete-to-end
ctrl-u        delete-to-start
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	// Verify key code bindings
	keycodeTests := []struct {
		key    twin.KeyCode
		action InputAction
	}{
		{twin.KeyLeft, CursorLeft},
		{twin.KeyRight, CursorRight},
		{twin.KeyHome, CursorHome},
		{twin.KeyEnd, CursorEnd},
		{twin.KeyBackspace, Backspace},
		{twin.KeyDelete, Delete},
	}

	for _, tt := range keycodeTests {
		if action, ok := bindings.Input.KeyCodeBindings[tt.key]; !ok || action != tt.action {
			t.Errorf("Expected key %v to map to action %v, got %v (found: %v)", tt.key, tt.action, action, ok)
		}
	}

	// Verify rune bindings
	runeTests := []struct {
		r      rune
		action InputAction
	}{
		{'\x01', CursorHome},    // ctrl-a
		{'\x05', CursorEnd},     // ctrl-e
		{'\x02', CursorLeft},    // ctrl-b
		{'\x06', CursorRight},   // ctrl-f
		{'\x0b', DeleteToEnd},   // ctrl-k
		{'\x15', DeleteToStart}, // ctrl-u
	}

	for _, tt := range runeTests {
		if action, ok := bindings.Input.RuneBindings[tt.r]; !ok || action != tt.action {
			t.Errorf("Expected rune %q (%#x) to map to action %v, got %v (found: %v)", tt.r, tt.r, tt.action, action, ok)
		}
	}
}

// TestParserInputPerKeyMerge tests that user-specified input bindings overlay
// defaults per-key, leaving all other input defaults intact.
func TestParserInputPerKeyMerge(t *testing.T) {
	input := `
[input]
left   cursor-right
right  cursor-left
ctrl-a cursor-end
ctrl-e cursor-home
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(input), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("Got unexpected warnings: %v", warnings)
	}

	// Overridden bindings
	if action, ok := bindings.Input.KeyCodeBindings[twin.KeyLeft]; !ok || action != CursorRight {
		t.Errorf("Expected left to map to CursorRight, got %v", action)
	}
	if action, ok := bindings.Input.KeyCodeBindings[twin.KeyRight]; !ok || action != CursorLeft {
		t.Errorf("Expected right to map to CursorLeft, got %v", action)
	}
	if action, ok := bindings.Input.RuneBindings['\x01']; !ok || action != CursorEnd {
		t.Errorf("Expected ctrl-a to map to CursorEnd, got %v", action)
	}
	if action, ok := bindings.Input.RuneBindings['\x05']; !ok || action != CursorHome {
		t.Errorf("Expected ctrl-e to map to CursorHome, got %v", action)
	}

	// Unmentioned input defaults must be intact.
	defaults := DefaultModeBindings()
	if action, ok := bindings.Input.KeyCodeBindings[twin.KeyHome]; !ok || action != defaults.Input.KeyCodeBindings[twin.KeyHome] {
		t.Error("Input Home binding lost")
	}
	if action, ok := bindings.Input.RuneBindings['\x0b']; !ok || action != defaults.Input.RuneBindings['\x0b'] {
		t.Error("Input ctrl-k binding lost")
	}
}

// TestDefaultKeybindingsTextIncludesInput tests that default text includes [input] section
func TestDefaultKeybindingsTextIncludesInput(t *testing.T) {
	text := DefaultKeybindingsText()

	if !strings.Contains(text, "[input]") {
		t.Error("DefaultKeybindingsText missing [input] section")
	}

	// Verify some expected bindings are present
	expectedBindings := []string{
		"left",
		"cursor-left",
		"ctrl-a",
		"cursor-home",
		"delete-to-end",
	}

	for _, expected := range expectedBindings {
		if !strings.Contains(text, expected) {
			t.Errorf("DefaultKeybindingsText missing expected text %q", expected)
		}
	}
}

// TestInputBindingsRoundTrip tests that input bindings round-trip through serialization
func TestInputBindingsRoundTrip(t *testing.T) {
	text := DefaultKeybindingsText()

	// Write and re-parse
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "keybindings")
	err := os.WriteFile(tmpFile, []byte(text), 0644)
	if err != nil {
		t.Fatalf("Failed to write temp file: %v", err)
	}

	bindings, warnings, err := ParseKeybindingsFile(tmpFile)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(warnings) > 0 {
		t.Errorf("Got warnings parsing default keybindings: %v", warnings)
	}

	// Verify default input bindings are present
	defaults := DefaultModeBindings()

	if len(bindings.Input.KeyCodeBindings) == 0 {
		t.Error("Input KeyCodeBindings is empty after round-trip")
	}

	if len(bindings.Input.RuneBindings) == 0 {
		t.Error("Input RuneBindings is empty after round-trip")
	}

	// Spot check a few bindings
	if action, ok := bindings.Input.KeyCodeBindings[twin.KeyLeft]; !ok {
		t.Error("KeyLeft binding missing after round-trip")
	} else if expected := defaults.Input.KeyCodeBindings[twin.KeyLeft]; action != expected {
		t.Errorf("KeyLeft action mismatch: got %v, want %v", action, expected)
	}

	if action, ok := bindings.Input.RuneBindings['\x01']; !ok {
		t.Error("Ctrl-A binding missing after round-trip")
	} else if expected := defaults.Input.RuneBindings['\x01']; action != expected {
		t.Errorf("Ctrl-A action mismatch: got %v, want %v", action, expected)
	}
}

func TestKeyForAction(t *testing.T) {
	defaults := DefaultModeBindings()

	// Default colon-command bindings: n→NextFile, p→PreviousFile, x→FirstFile
	if got := keyForAction(defaults.ColonCommand, NextFile); got != "n" {
		t.Errorf("NextFile key: got %q, want %q", got, "n")
	}
	if got := keyForAction(defaults.ColonCommand, PreviousFile); got != "p" {
		t.Errorf("PreviousFile key: got %q, want %q", got, "p")
	}
	if got := keyForAction(defaults.ColonCommand, FirstFile); got != "x" {
		t.Errorf("FirstFile key: got %q, want %q", got, "x")
	}

	// Unbound action returns "?"
	if got := keyForAction(defaults.ColonCommand, ScrollUp); got != "?" {
		t.Errorf("Unbound action: got %q, want %q", got, "?")
	}

	// Custom rune binding overrides the default
	custom := DefaultModeBindings()
	custom.ColonCommand.RuneBindings['j'] = NextFile
	delete(custom.ColonCommand.RuneBindings, 'n')
	if got := keyForAction(custom.ColonCommand, NextFile); got != "j" {
		t.Errorf("Custom rune binding: got %q, want %q", got, "j")
	}

	// KeyCode binding is found when no rune binding exists
	keyCodeOnly := DefaultModeBindings()
	delete(keyCodeOnly.ColonCommand.RuneBindings, 'n')
	keyCodeOnly.ColonCommand.KeyCodeBindings[twin.KeyEnter] = NextFile
	if got := keyForAction(keyCodeOnly.ColonCommand, NextFile); got != "enter" {
		t.Errorf("KeyCode binding: got %q, want %q", got, "enter")
	}
}

func TestRuneDisplayName(t *testing.T) {
	tests := []struct {
		r    rune
		want string
	}{
		{'n', "n"},
		{'N', "N"},
		{' ', "space"},
		{'\t', "tab"},
		{1, "ctrl-a"},  // ctrl-a
		{14, "ctrl-n"}, // ctrl-n
		{26, "ctrl-z"}, // ctrl-z
	}
	for _, tt := range tests {
		if got := runeDisplayName(tt.r); got != tt.want {
			t.Errorf("runeDisplayName(%q): got %q, want %q", tt.r, got, tt.want)
		}
	}
}

func TestKeyCodeDisplayName(t *testing.T) {
	// escape has one alias; verify a canonical name is returned.
	if got := keyCodeDisplayName(twin.KeyEscape); got == "" {
		t.Error("keyCodeDisplayName(KeyEscape) returned empty string")
	}
	// enter has one alias too.
	if got := keyCodeDisplayName(twin.KeyEnter); got == "" {
		t.Error("keyCodeDisplayName(KeyEnter) returned empty string")
	}
	// pageup / pgup: both map to KeyPgUp; the shorter "pgup" should win.
	if got := keyCodeDisplayName(twin.KeyPgUp); got != "pgup" {
		t.Errorf("keyCodeDisplayName(KeyPgUp): got %q, want %q", got, "pgup")
	}
	// pagedown / pgdown: both map to KeyPgDown; "pgdown" should win.
	if got := keyCodeDisplayName(twin.KeyPgDown); got != "pgdown" {
		t.Errorf("keyCodeDisplayName(KeyPgDown): got %q, want %q", got, "pgdown")
	}
}
