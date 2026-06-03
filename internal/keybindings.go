package internal

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/walles/moor/v2/twin"
)

// Action represents actions available across all modes
type Action int

const (
	NoAction Action = iota
	// Navigation
	Quit
	Reload
	Edit
	Help
	ToggleStatusBar
	ScrollUp
	ScrollDown
	ScrollLeft
	ScrollRight
	ScrollLeft1
	ScrollRight1
	ScrollTop
	ScrollBottom
	ScrollHome
	ScrollPageUp
	ScrollPageDown
	ScrollHalfPageUp
	ScrollHalfPageDown
	// Search
	SearchForward
	SearchBackward
	NextSearchHit
	PreviousSearchHit
	// Modes
	Filter
	GotoLine
	ColonCommand
	Mark
	JumpToMark
	// Display
	ToggleWrap
	CycleTabSize
	// Input modes
	Accept
	Cancel
	HistoryPrevious
	HistoryNext
	// Goto specific
	GotoTop
	// Colon command specific
	NextFile
	PreviousFile
	FirstFile
)

// InputAction represents actions available for input box editing
type InputAction int

const (
	NoInputAction InputAction = iota
	CursorLeft
	CursorRight
	CursorHome
	CursorEnd
	Backspace
	Delete
	DeleteToEnd
	DeleteToStart
)

// KeyBindings holds the key bindings for a specific mode
type KeyBindings[A any] struct {
	KeyCodeBindings map[twin.KeyCode]A
	RuneBindings    map[rune]A
}

// ModeBindings holds key bindings for all modes
type ModeBindings struct {
	Viewing      KeyBindings[Action]
	NotFound     KeyBindings[Action]
	Search       KeyBindings[Action]
	Filter       KeyBindings[Action]
	GotoLine     KeyBindings[Action]
	ColonCommand KeyBindings[Action]
	Mark         KeyBindings[Action]
	JumpToMark   KeyBindings[Action]
	Input        KeyBindings[InputAction]
}

// bindingEntry represents a single key-to-action binding
type bindingEntry[A any] struct {
	key    string
	action A
}

// Action name tables
var actionNames = map[Action]string{
	NoAction:           "noaction",
	Quit:               "quit",
	Reload:             "reload",
	Edit:               "edit",
	Help:               "help",
	ToggleStatusBar:    "toggle-statusbar",
	ScrollUp:           "scroll-up",
	ScrollDown:         "scroll-down",
	ScrollTop:          "scroll-top",
	ScrollBottom:       "scroll-bottom",
	ScrollPageDown:     "scroll-page-down",
	ScrollPageUp:       "scroll-page-up",
	ScrollHalfPageDown: "scroll-half-page-down",
	ScrollHalfPageUp:   "scroll-half-page-up",
	ScrollRight:        "scroll-right",
	ScrollLeft:         "scroll-left",
	ScrollRight1:       "scroll-right-1",
	ScrollLeft1:        "scroll-left-1",
	SearchForward:      "search-forward",
	SearchBackward:     "search-backward",
	Filter:             "filter",
	GotoLine:           "goto-line",
	ColonCommand:       "colon-command",
	NextSearchHit:      "next-search-hit",
	PreviousSearchHit:  "previous-search-hit",
	Mark:               "mark",
	JumpToMark:         "jump-to-mark",
	ToggleWrap:         "toggle-wrap",
	CycleTabSize:       "cycle-tab-size",
	ScrollHome:         "scroll-home",
	Accept:             "accept",
	Cancel:             "cancel",
	HistoryPrevious:    "history-previous",
	HistoryNext:        "history-next",
	GotoTop:            "goto-top",
	NextFile:           "next-file",
	PreviousFile:       "previous-file",
	FirstFile:          "first-file",
}

// Input action name tables
var inputActionNames = map[InputAction]string{
	NoInputAction: "noaction",
	CursorLeft:    "cursor-left",
	CursorRight:   "cursor-right",
	CursorHome:    "cursor-home",
	CursorEnd:     "cursor-end",
	Backspace:     "backspace",
	Delete:        "delete",
	DeleteToEnd:   "delete-to-end",
	DeleteToStart: "delete-to-start",
}

// Reverse maps (built lazily)
var (
	actionNamesReverse      map[string]Action
	inputActionNamesReverse map[string]InputAction
	reverseMapOnce          sync.Once
)

func buildReverseMaps() {
	actionNamesReverse = make(map[string]Action)
	for action, name := range actionNames {
		actionNamesReverse[name] = action
	}

	inputActionNamesReverse = make(map[string]InputAction)
	for action, name := range inputActionNames {
		inputActionNamesReverse[name] = action
	}
}

// commonActions is derived from the keys of commonActionHandlers and lists
// every Action that is common across all modes, excluding NoAction.
var commonActions = func() []Action {
	actions := make([]Action, 0, len(commonActionHandlers))
	for a := range commonActionHandlers {
		actions = append(actions, a)
	}
	return actions
}()

// GetValidActionNames returns a sorted list of valid action names for a given
// section, derived from that section's default entries plus any actions it
// inherits at runtime (common actions or all viewing actions).
func GetValidActionNames(section string) []string {
	// actionsFrom extracts the Action values from a slice of binding entries.
	actionsFrom := func(entries []bindingEntry[Action]) []Action {
		actions := make([]Action, len(entries))
		for i, e := range entries {
			actions[i] = e.action
		}
		return actions
	}

	// collect deduplicates action names across one or more Action slices,
	// skips "noaction", and returns the result sorted.
	collect := func(groups ...[]Action) []string {
		seen := make(map[Action]bool)
		var names []string
		for _, group := range groups {
			for _, a := range group {
				if seen[a] {
					continue
				}
				seen[a] = true
				if name, ok := actionNames[a]; ok && name != "noaction" {
					names = append(names, name)
				}
			}
		}
		sort.Strings(names)
		return names
	}

	for _, s := range actionSections {
		if section == s.name {
			return collect(actionsFrom(s.entries), actionsFrom(s.inherits), commonActions)
		}
	}

	if section == "input" {
		// Input uses a separate action type; no commonActions apply.
		seen := make(map[InputAction]bool)
		var names []string
		for _, entry := range defaultInputEntries {
			if seen[entry.action] {
				continue
			}
			seen[entry.action] = true
			if name, ok := inputActionNames[entry.action]; ok && name != "noaction" {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		return names
	}

	return nil
}

// Default bindings - source of truth
var defaultViewingEntries = []bindingEntry[Action]{
	{"escape", Quit},
	{"q", Quit},
	{"r", Reload},
	{"v", Edit},
	{"h", Help},
	{"=", ToggleStatusBar},
	{"k", ScrollUp},
	{"y", ScrollUp},
	{"ctrl-p", ScrollUp},
	{"up", ScrollUp},
	{"j", ScrollDown},
	{"e", ScrollDown},
	{"ctrl-n", ScrollDown},
	{"down", ScrollDown},
	{"enter", ScrollDown},
	{"<", ScrollTop},
	{"home", ScrollTop},
	{">", ScrollBottom},
	{"G", ScrollBottom},
	{"end", ScrollBottom},
	{"f", ScrollPageDown},
	{"space", ScrollPageDown},
	{"ctrl-f", ScrollPageDown},
	{"pagedown", ScrollPageDown},
	{"b", ScrollPageUp},
	{"ctrl-b", ScrollPageUp},
	{"pageup", ScrollPageUp},
	{"u", ScrollHalfPageUp},
	{"ctrl-u", ScrollHalfPageUp},
	{"d", ScrollHalfPageDown},
	{"ctrl-d", ScrollHalfPageDown},
	{"right", ScrollRight},
	{"left", ScrollLeft},
	{"alt-right", ScrollRight1},
	{"alt-left", ScrollLeft1},
	{"/", SearchForward},
	{"?", SearchBackward},
	{"&", Filter},
	{"g", GotoLine},
	{":", ColonCommand},
	{"n", NextSearchHit},
	{"p", PreviousSearchHit},
	{"N", PreviousSearchHit},
	{"m", Mark},
	{"'", JumpToMark},
	{"w", ToggleWrap},
	{"ctrl-t", CycleTabSize},
	{"ctrl-a", ScrollHome},
}

var defaultNotFoundEntries = []bindingEntry[Action]{
	{"escape", Cancel},
}

var defaultSearchEntries = []bindingEntry[Action]{
	{"enter", Accept},
	{"escape", Cancel},
	{"ctrl-c", Cancel},
	{"up", HistoryPrevious},
	{"down", HistoryNext},
	{"pageup", ScrollPageUp},
	{"pagedown", ScrollPageDown},
}

var defaultFilterEntries = []bindingEntry[Action]{
	{"enter", Accept},
	{"escape", Cancel},
	{"up", ScrollUp},
	{"down", ScrollDown},
	{"pageup", ScrollPageUp},
	{"pagedown", ScrollPageDown},
}

var defaultGotoLineEntries = []bindingEntry[Action]{
	{"enter", Accept},
	{"escape", Cancel},
	{"g", GotoTop},
	{"q", Cancel},
}

var defaultColonCommandEntries = []bindingEntry[Action]{
	{"escape", Cancel},
	{"q", Cancel},
	{"n", NextFile},
	{"p", PreviousFile},
	{"x", FirstFile},
}

var defaultMarkEntries = []bindingEntry[Action]{
	{"escape", Cancel},
}

var defaultJumpToMarkEntries = []bindingEntry[Action]{
	{"enter", Cancel},
	{"escape", Cancel},
}

// sectionInfo ties a section name to its default Action entries and a pointer
// accessor for the corresponding field in ModeBindings.  This is the single
// source of truth for the section-name → entries mapping
type sectionInfo struct {
	name     string
	entries  []bindingEntry[Action]
	field    func(*ModeBindings) *KeyBindings[Action]
	inherits []bindingEntry[Action] // runtime fallback entries whose actions are also valid here
}

// actionSections is the ordered list of all non-input sections.
var actionSections = []sectionInfo{
	{"viewing", defaultViewingEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.Viewing }, nil},
	{"not-found", defaultNotFoundEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.NotFound }, defaultViewingEntries},
	{"search", defaultSearchEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.Search }, nil},
	{"filter", defaultFilterEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.Filter }, nil},
	{"goto-line", defaultGotoLineEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.GotoLine }, nil},
	{"colon-command", defaultColonCommandEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.ColonCommand }, nil},
	{"mark", defaultMarkEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.Mark }, nil},
	{"jump-to-mark", defaultJumpToMarkEntries, func(mb *ModeBindings) *KeyBindings[Action] { return &mb.JumpToMark }, nil},
}

var defaultInputEntries = []bindingEntry[InputAction]{
	{"left", CursorLeft},
	{"right", CursorRight},
	{"home", CursorHome},
	{"end", CursorEnd},
	{"backspace", Backspace},
	{"ctrl-h", Backspace}, // Some keyboards/terminals send 0x08 instead of backspace
	{"delete", Delete},
	{"ctrl-a", CursorHome},
	{"ctrl-e", CursorEnd},
	{"ctrl-b", CursorLeft},
	{"ctrl-f", CursorRight},
	{"ctrl-k", DeleteToEnd},
	{"ctrl-u", DeleteToStart},
}

var (
	defaultModeBindingsCache *ModeBindings
	defaultModeBindingsOnce  sync.Once
)

// DefaultModeBindings returns the default key bindings for all modes
func DefaultModeBindings() ModeBindings {
	defaultModeBindingsOnce.Do(func() {
		bindings := ModeBindings{
			Input: buildKeyBindings(defaultInputEntries),
		}
		for _, s := range actionSections {
			*s.field(&bindings) = buildKeyBindings(s.entries)
		}
		defaultModeBindingsCache = &bindings
	})
	result := ModeBindings{
		Input: cloneKeyBindings(defaultModeBindingsCache.Input),
	}
	for _, s := range actionSections {
		*s.field(&result) = cloneKeyBindings(*s.field(defaultModeBindingsCache))
	}
	return result
}

// EmptyModeBindings returns a ModeBindings where every section has empty
// (non-nil) maps and no bindings.  It is the counterpart of DefaultModeBindings
// and is used, for example, to implement the !clear directive in keybinding
// files.
func EmptyModeBindings() ModeBindings {
	mb := ModeBindings{
		Input: emptyKeyBindings[InputAction](),
	}
	for _, s := range actionSections {
		*s.field(&mb) = emptyKeyBindings[Action]()
	}
	return mb
}

// emptyKeyBindings returns a KeyBindings with empty, non-nil maps.
func emptyKeyBindings[A any]() KeyBindings[A] {
	return KeyBindings[A]{
		KeyCodeBindings: make(map[twin.KeyCode]A),
		RuneBindings:    make(map[rune]A),
	}
}

// cloneKeyBindings returns a shallow copy of kb with independently cloned maps,
// so callers cannot mutate the shared cache.
func cloneKeyBindings[A any](kb KeyBindings[A]) KeyBindings[A] {
	return KeyBindings[A]{
		KeyCodeBindings: maps.Clone(kb.KeyCodeBindings),
		RuneBindings:    maps.Clone(kb.RuneBindings),
	}
}

// buildKeyBindings converts a slice of binding entries to KeyBindings
func buildKeyBindings[A any](entries []bindingEntry[A]) KeyBindings[A] {
	kb := KeyBindings[A]{
		KeyCodeBindings: make(map[twin.KeyCode]A),
		RuneBindings:    make(map[rune]A),
	}

	for _, entry := range entries {
		keyCode, isKeyCode := parseKeyCodeName(entry.key)
		if isKeyCode {
			kb.KeyCodeBindings[keyCode] = entry.action
			continue
		}

		r, isRune := parseRuneKeyName(entry.key)
		if isRune {
			kb.RuneBindings[r] = entry.action
			continue
		}

		// This should never happen with valid default entries
		panic(fmt.Sprintf("Invalid key name in defaults: %q", entry.key))
	}

	return kb
}

var keyCodeByName = map[string]twin.KeyCode{
	"escape":    twin.KeyEscape,
	"enter":     twin.KeyEnter,
	"backspace": twin.KeyBackspace,
	"delete":    twin.KeyDelete,
	"up":        twin.KeyUp,
	"down":      twin.KeyDown,
	"left":      twin.KeyLeft,
	"right":     twin.KeyRight,
	"home":      twin.KeyHome,
	"end":       twin.KeyEnd,
	"pageup":    twin.KeyPgUp,
	"pgup":      twin.KeyPgUp,
	"pagedown":  twin.KeyPgDown,
	"pgdown":    twin.KeyPgDown,
	"alt-up":    twin.KeyAltUp,
	"alt-down":  twin.KeyAltDown,
	"alt-left":  twin.KeyAltLeft,
	"alt-right": twin.KeyAltRight,
}

func parseKeyCodeName(name string) (twin.KeyCode, bool) {
	lower := strings.ToLower(name)
	kc, ok := keyCodeByName[lower]
	return kc, ok
}

// parseRuneKeyName parses a key name into a rune
func parseRuneKeyName(name string) (rune, bool) {
	// Control characters: ctrl-a through ctrl-z (lowercase only)
	if strings.HasPrefix(name, "ctrl-") && len(name) == 6 {
		ch := name[5] // Character after "ctrl-"
		if ch >= 'a' && ch <= 'z' {
			return rune(ch - 'a' + 1), true
		}
	}

	// Special rune names (case-insensitive for convenience)
	var runeByName = map[string]rune{
		"space": ' ',
		"tab":   '\t',
		"\\t":   '\t',
		"\\n":   '\n',
	}
	lower := strings.ToLower(name)
	if r, ok := runeByName[lower]; ok {
		return r, true
	}

	// Single character
	runes := []rune(name)
	if len(runes) == 1 {
		return runes[0], true
	}

	return 0, false
}

// serializeSection converts a slice of bindings to text format
func serializeSection[A comparable](entries []bindingEntry[A], actionNames map[A]string) string {
	var result strings.Builder

	maxKeyLen := 0
	for _, entry := range entries {
		if len(entry.key) > maxKeyLen {
			maxKeyLen = len(entry.key)
		}
	}

	for _, entry := range entries {
		actionName := actionNames[entry.action]
		padding := strings.Repeat(" ", maxKeyLen-len(entry.key)+2)
		fmt.Fprintf(&result, "%s%s%s\n", entry.key, padding, actionName)
	}

	return result.String()
}

// DefaultKeybindingsText returns the default keybindings as a text file
func DefaultKeybindingsText() string {
	var result strings.Builder

	result.WriteString("# Keybindings for moor\n")
	result.WriteString("# Lines starting with # are comments. Blank lines are ignored.\n")
	result.WriteString("# Format: <key>  <action>\n")
	result.WriteString("#\n")
	result.WriteString("# Modifiers: alt-up, alt-down, alt-left, alt-right\n")
	result.WriteString("# Control:   ctrl-a through ctrl-z (lowercase only, e.g., ctrl-d, ctrl-u)\n")
	result.WriteString("# Special:   escape, enter, backspace, delete, up, down, left, right,\n")
	result.WriteString("#            home, end, pageup, pagedown\n")
	result.WriteString("# Letters:   Case-sensitive ('a' is different from 'A')\n")
	result.WriteString("# Space:     space\n")
	result.WriteString("#\n")
	result.WriteString("# Use 'noaction' to disable a key binding\n")
	result.WriteString("\n")

	result.WriteString("# Clears the current key bindings\n")
	result.WriteString("!clear\n")
	result.WriteString("\n")

	for _, s := range actionSections {
		result.WriteString("[" + s.name + "]\n")
		result.WriteString(serializeSection(s.entries, actionNames))
		result.WriteString("\n")
	}

	result.WriteString("[input]\n")
	result.WriteString(serializeSection(defaultInputEntries, inputActionNames))

	return result.String()
}

// keyForAction returns a human-readable key name for the first key bound to the
// given action in kb. Rune bindings are preferred over key-code bindings because
// they produce shorter, more legible labels (e.g. "n" instead of "enter").
// When multiple keys share the same action the result is the lexicographically
// smallest match, so the output is deterministic. Returns "?" when no binding
// is found.
func keyForAction(kb KeyBindings[Action], action Action) string {
	// Collect matching key-code keys, sort for determinism.
	var names []string
	for kc, a := range kb.KeyCodeBindings {
		if a == action {
			names = append(names, keyCodeDisplayName(kc))
		}
	}
	if len(names) > 0 {
		sort.Strings(names)
		return names[0]
	}

	// Collect matching rune keys, sort for determinism.
	var runes []rune
	for r, a := range kb.RuneBindings {
		if a == action {
			runes = append(runes, r)
		}
	}
	if len(runes) > 0 {
		sort.Slice(runes, func(i, j int) bool {
			return runeDisplayName(runes[i]) < runeDisplayName(runes[j])
		})
		return runeDisplayName(runes[0])
	}

	return "?"
}

// runeDisplayName returns a short, human-readable label for a rune key.
func runeDisplayName(r rune) string {
	// Check named specials before the ctrl range so that e.g. tab (rune 9)
	// is shown as "tab" rather than "ctrl-i".
	switch r {
	case ' ':
		return "space"
	case '\t':
		return "tab"
	}
	if r >= 1 && r <= 26 {
		return fmt.Sprintf("ctrl-%c", 'a'+r-1)
	}
	return string(r)
}

// keyCodeDisplayName returns a human-readable label for a KeyCode by
// reverse-looking up keyCodeByName. Falls back to a numeric representation.
func keyCodeDisplayName(kc twin.KeyCode) string {
	// Prefer the shorter / more canonical name when multiple aliases exist.
	best := ""
	for name, code := range keyCodeByName {
		if code == kc {
			if best == "" || len(name) < len(best) || (len(name) == len(best) && name < best) {
				best = name
			}
		}
	}
	if best != "" {
		return best
	}
	return fmt.Sprintf("key(%d)", kc)
}

// DefaultKeybindingsPath returns the default path for the keybindings file
func DefaultKeybindingsPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home := os.Getenv("HOME")
		if home == "" {
			// Fallback if HOME is not set (unlikely on Unix systems)
			return ""
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "moor", "keybindings")
}
