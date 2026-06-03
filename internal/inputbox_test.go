package internal

import (
	"testing"

	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

func TestInsertAndBackspace(t *testing.T) {
	screen := twin.NewFakeScreen(40, 2)
	b := NewInputBox(INPUTBOX_ACCEPT_ALL, DefaultModeBindings().Input)

	assert.Assert(t, b.handleRune('a'))
	assert.Assert(t, b.handleRune('b'))
	assert.Assert(t, b.handleRune('c'))
	assert.Equal(t, "abc", b.text)

	// Backspace
	b.backspace()
	assert.Equal(t, "ab", b.text)

	// Draw and inspect status line
	b.draw(screen, "", "P: ")
	row := rowToString(screen.GetRow(1))
	assert.Equal(t, "P: ab", row)
}

func TestCursorMovementAndInsertDelete(t *testing.T) {
	screen := twin.NewFakeScreen(80, 2)
	b := NewInputBox(INPUTBOX_ACCEPT_ALL, DefaultModeBindings().Input)
	b.handleRune('a')
	b.handleRune('b')
	b.handleRune('c')
	assert.Equal(t, "abc", b.text)

	// Move left twice, insert 'X' between a and b
	b.moveCursorLeft()
	b.moveCursorLeft()
	assert.Assert(t, b.handleRune('X'))
	assert.Equal(t, "aXbc", b.text)

	// Delete at cursor (cursor is after X)
	b.delete()
	assert.Equal(t, "aXc", b.text)

	// Move home and insert
	b.moveCursorHome()
	assert.Assert(t, b.handleRune('S'))
	assert.Equal(t, "SaXc", b.text)

	// Move end and append
	b.moveCursorEnd()
	assert.Assert(t, b.handleRune('E'))
	assert.Equal(t, "SaXcE", b.text)

	b.draw(screen, "", "G: ")
	row := rowToString(screen.GetRow(1))
	assert.Equal(t, "G: SaXcE", row)
}

func TestCtrlHBackspace(t *testing.T) {
	// Some keyboards/terminals send 0x08 (ctrl-h) instead of the backspace key.
	// Regression test: handleRune('\x08') must behave identically to a real backspace.
	b := NewInputBox(INPUTBOX_ACCEPT_ALL, DefaultModeBindings().Input)

	assert.Assert(t, b.handleRune('x'))
	assert.Assert(t, b.handleRune('y'))
	assert.Equal(t, "xy", b.text)

	// ctrl-h should delete the last character
	assert.Assert(t, b.handleRune('\x08'))
	assert.Equal(t, "x", b.text)
}

func TestAcceptPositiveNumbers(t *testing.T) {
	b := NewInputBox(INPUTBOX_ACCEPT_POSITIVE_NUMBERS, DefaultModeBindings().Input)
	assert.Assert(t, b.handleRune('1'))
	assert.Assert(t, !b.handleRune('a'))
	assert.Assert(t, b.handleRune('2'))
	assert.Equal(t, "12", b.text)
}

func TestUnicodeRunes(t *testing.T) {
	screen := twin.NewFakeScreen(80, 2)
	b := NewInputBox(INPUTBOX_ACCEPT_ALL, DefaultModeBindings().Input)
	// Insert a CJK char and an emoji
	assert.Assert(t, b.handleRune('午'))
	assert.Assert(t, b.handleRune('🧐'))
	assert.Equal(t, "午🧐", b.text)

	// Backspace should remove the emoji
	b.backspace()
	assert.Equal(t, "午", b.text)

	// Insert another wide char at start
	b.moveCursorHome()
	assert.Assert(t, b.handleRune('你'))
	assert.Equal(t, "你午", b.text)

	b.draw(screen, "", "U: ")
	row := rowToString(screen.GetRow(1))
	// We expect prompt + two runes
	assert.Equal(t, "U: 你午", row)
}
