package internal

import (
	"unicode"

	"github.com/walles/twin"
)

type InputBoxOnTextChanged func(text string)

type AcceptMode int

const (
	INPUTBOX_ACCEPT_ALL AcceptMode = iota
	INPUTBOX_ACCEPT_POSITIVE_NUMBERS
)

type InputBox struct {
	text string

	// accept controls what input is accepted. Use the INPUTBOX_ACCEPT_*
	// constants defined above.
	accept AcceptMode

	// cursorPos is the insertion point in runes (0 == before first rune,
	// len(runes) == after last rune).
	cursorPos int

	// onTextChanged is an optional callback which is triggered when the text
	// of the InputBox changes.
	onTextChanged InputBoxOnTextChanged
}

// draw renders the input box at the bottom line of the screen, showing a
// simple prompt and the current text with a reverse attribute cursor.
func (b *InputBox) draw(screen twin.Screen, keys_help string, prompt string) {
	width, height := screen.Size()
	pos := 0

	// Draw the prompt first
	for _, ch := range prompt {
		pos += screen.SetCell(pos, height-1, twin.NewStyledRune(ch, twin.StyleDefault))
	}

	// Work with runes for cursor correctness
	textRunes := []rune(b.text)
	if b.cursorPos < 0 {
		b.cursorPos = 0
	}
	if b.cursorPos > len(textRunes) {
		b.cursorPos = len(textRunes)
	}

	// Draw left side (before cursor)
	for i, ch := range textRunes {
		if i == b.cursorPos {
			break
		}
		pos += screen.SetCell(pos, height-1, twin.NewStyledRune(ch, twin.StyleDefault))
	}

	// If cursor is on a rune, invert that rune. If cursor is at the end,
	// show an inverted blank cell.
	if b.cursorPos < len(textRunes) {
		pos += screen.SetCell(pos, height-1, twin.NewStyledRune(textRunes[b.cursorPos], twin.StyleDefault.WithAttr(twin.AttrReverse)))

		// Draw right side after the cursor rune
		for i := b.cursorPos + 1; i < len(textRunes); i++ {
			pos += screen.SetCell(pos, height-1, twin.NewStyledRune(textRunes[i], twin.StyleDefault))
		}
	} else {
		// Cursor at end -> reverse blank
		pos += screen.SetCell(pos, height-1, twin.NewStyledRune(' ', twin.StyleDefault.WithAttr(twin.AttrReverse)))
	}

	afterTextPos := pos

	// Clear the rest of the line
	for pos < width {
		pos += screen.SetCell(pos, height-1, twin.NewStyledRune(' ', twin.StyleDefault))
	}

	// Draw help on the right
	if len(keys_help) > 0 {
		renderedHelp := renderHelpText(keys_help)
		helpStart := width - len(renderedHelp)
		if helpStart > afterTextPos {
			// Draw the help text
			pos = width - len(renderedHelp)
			for _, cell := range renderedHelp {
				pos += screen.SetCell(pos, height-1, cell)
			}
			screen.SetCell(pos, height-1, twin.NewStyledRune(' ', theme.statusbar))
		}
	}
}

func (b *InputBox) setText(text string) {
	b.text = text
	b.moveCursorEnd()
	if b.onTextChanged != nil {
		b.onTextChanged(b.text)
	}
}

// handleRune appends runes to the text of the InputBox and returns if those have been processed.
// (Some keyboards send 0x08 instead of backspace, so we support it here too).
func (b *InputBox) handleRune(char rune) bool {
	if char == '\x08' {
		b.backspace()
		return true
	}
	if char == '\x01' {
		// Ctrl-A, move cursor to start
		b.moveCursorHome()
		return true
	}
	if char == '\x05' {
		// Ctrl-E, move cursor to end
		b.moveCursorEnd()
		return true
	}
	if char == '\x02' {
		// Ctrl-B, move cursor left
		b.moveCursorLeft()
		return true
	}
	if char == '\x06' {
		// Ctrl-F, move cursor right
		b.moveCursorRight()
		return true
	}
	if char == '\x0b' {
		// Ctrl-K, delete to end of line
		b.deleteToEnd()
		return true
	}
	if char == '\x15' {
		// Ctrl-U, delete to start of line
		b.deleteToStart()
		return true
	}

	// If configured to accept numbers only, drop any non-digit rune.
	if b.accept == INPUTBOX_ACCEPT_POSITIVE_NUMBERS {
		if !unicode.IsDigit(char) {
			return false
		}
	}

	// Insert at cursor position
	runes := []rune(b.text)
	if b.cursorPos < 0 {
		b.cursorPos = 0
	}
	if b.cursorPos > len(runes) {
		b.cursorPos = len(runes)
	}

	// Build a new rune slice with the inserted rune
	newRunes := make([]rune, 0, len(runes)+1)
	newRunes = append(newRunes, runes[:b.cursorPos]...)
	newRunes = append(newRunes, char)
	if b.cursorPos < len(runes) {
		newRunes = append(newRunes, runes[b.cursorPos:]...)
	}
	b.text = string(newRunes)
	b.cursorPos++

	// finally let's tell someone that the text has changed
	if b.onTextChanged != nil {
		b.onTextChanged(b.text)
	}
	return true
}

// handleKey processes special keys like backspace, delete, arrow keys, home and end.
// Returns true if the key was processed, false otherwise.
func (b *InputBox) handleKey(key twin.KeyCode) bool {
	switch key {
	case twin.KeyLeft:
		b.moveCursorLeft()
		return true

	case twin.KeyRight:
		b.moveCursorRight()
		return true

	case twin.KeyHome:
		b.moveCursorHome()
		return true

	case twin.KeyEnd:
		b.moveCursorEnd()
		return true

	case twin.KeyBackspace:
		b.backspace()
		return true

	case twin.KeyDelete:
		b.delete()
		return true
	}

	return false
}

// moveCursorLeft moves the cursor one rune to the left.
func (b *InputBox) moveCursorLeft() {
	if b.cursorPos > 0 {
		b.cursorPos--
	}
}

// moveCursorRight moves the cursor one rune to the right.
func (b *InputBox) moveCursorRight() {
	if b.cursorPos < len([]rune(b.text)) {
		b.cursorPos++
	}
}

// moveCursorHome moves the cursor to the start of the text.
func (b *InputBox) moveCursorHome() {
	b.cursorPos = 0
}

// moveCursorEnd moves the cursor to the end of the text.
func (b *InputBox) moveCursorEnd() {
	b.cursorPos = len([]rune(b.text))
}

func (b *InputBox) deleteToEnd() {
	b.text = string([]rune(b.text)[:b.cursorPos])
	if b.onTextChanged != nil {
		b.onTextChanged(b.text)
	}
}

func (b *InputBox) deleteToStart() {
	b.text = string([]rune(b.text)[b.cursorPos:])
	b.cursorPos = 0
	if b.onTextChanged != nil {
		b.onTextChanged(b.text)
	}
}

// backspace removes the rune before the cursor and moves the cursor left.
func (b *InputBox) backspace() {
	runes := []rune(b.text)
	if b.cursorPos > 0 && len(runes) > 0 {
		runes = append(runes[:b.cursorPos-1], runes[b.cursorPos:]...)
		b.cursorPos--
		b.text = string(runes)
		if b.onTextChanged != nil {
			b.onTextChanged(b.text)
		}
	}
}

// delete removes the rune at the cursor.
func (b *InputBox) delete() {
	runes := []rune(b.text)
	if b.cursorPos < len(runes) {
		runes = append(runes[:b.cursorPos], runes[b.cursorPos+1:]...)
		b.text = string(runes)
		if b.onTextChanged != nil {
			b.onTextChanged(b.text)
		}
	}
}
