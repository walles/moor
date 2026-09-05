package internal

import (
	"testing"

	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

func TestInsertAndBackspace(t *testing.T) {
	screen := twin.NewFakeScreen(40, 2)
	b := &InputBox{accept: INPUTBOX_ACCEPT_ALL}

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
	b := &InputBox{accept: INPUTBOX_ACCEPT_ALL}
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

func TestAcceptPositiveNumbers(t *testing.T) {
	b := &InputBox{accept: INPUTBOX_ACCEPT_POSITIVE_NUMBERS}
	assert.Assert(t, b.handleRune('1'))
	assert.Assert(t, !b.handleRune('a'))
	assert.Assert(t, b.handleRune('2'))
	assert.Equal(t, "12", b.text)
}

func TestUnicodeRunes(t *testing.T) {
	screen := twin.NewFakeScreen(80, 2)
	b := &InputBox{accept: INPUTBOX_ACCEPT_ALL}
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
