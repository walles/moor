//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/ptr"
)

// Paging input that doesn't fit on one screen happens on the alternate screen,
// so that the user's terminal contents are restored on exit.
func TestLongInputUsesAlternateScreen(t *testing.T) {
	for _, fromPipe := range []bool{false, true} {
		t.Run(fmt.Sprintf("fromPipe=%t", fromPipe), func(t *testing.T) {
			options := moorOptions{answerBackgroundQuery: true}
			if fromPipe {
				options.stdin = ptr.To(textLines(100))
			} else {
				options.args = []string{createTextFile(t, 100)}
			}

			session := startMoor(t, options)

			// The status bar, so we know a full screen has been painted
			session.waitFor(t, "100 lines")

			session.send(t, "q")
			session.wait(t)

			assertAltScreenPaired(t, session.captured())
		})
	}
}

// Launching an editor with "v" puts the user's terminal back the way it was for
// the editor, and takes it back again afterwards, without ever getting the
// alternate screen bookkeeping out of sync.
func TestEditorRoundTripKeepsAlternateScreenPaired(t *testing.T) {
	session := startMoor(t, moorOptions{
		answerBackgroundQuery: true,
		extraEnv:              []string{"EDITOR=" + createEditorStub(t)},
		args:                  []string{createTextFile(t, 100)},
	})

	// The status bar, so we know a full screen has been painted
	session.waitFor(t, "100 lines")

	session.send(t, "v")
	session.waitFor(t, editorStubMarker)

	// Moor puts the terminal back in raw mode before entering the alternate
	// screen again, so by the time we see that second entry our quit key will
	// be delivered as a single keypress rather than getting stuck in a line
	// buffer waiting for a newline.
	session.waitForCount(t, altScreenEnter, 2)

	session.send(t, "q")
	session.wait(t)

	captured := session.captured()
	assert.Equal(t, strings.Count(captured, altScreenEnter), 2,
		"Expected one alternate screen entry at startup and one after the editor:\n%s",
		humanizeEscapes(captured))
	assertAltScreenPaired(t, captured)
}

// Fails the test unless moor entered the alternate screen, left it exactly as
// many times as it entered it, and entered it before leaving it the first time.
//
// An unpaired ESC[?1049l is not harmless: DECRST 1049 also restores a saved
// cursor position, so it can teleport the user's cursor.
func assertAltScreenPaired(t *testing.T, captured string) {
	t.Helper()

	assert.Equal(t, strings.Count(captured, altScreenEnter), strings.Count(captured, altScreenLeave),
		"Unbalanced alternate screen switching:\n%s", humanizeEscapes(captured))

	assert.Assert(t, strings.Contains(captured, altScreenEnter),
		"Never entered the alternate screen:\n%s", humanizeEscapes(captured))

	assert.Assert(t, strings.Index(captured, altScreenEnter) < strings.Index(captured, altScreenLeave),
		"Left the alternate screen before entering it:\n%s", humanizeEscapes(captured))
}

// What createEditorStub()'s editor prints, and moor doesn't.
const editorStubMarker = "editor-stub-was-here"

// Creates a stand-in for $EDITOR that announces itself and exits immediately,
// and returns its path.
func createEditorStub(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "editor-stub")
	script := "#!/bin/sh\necho " + editorStubMarker + "\n"
	assert.NilError(t, os.WriteFile(path, []byte(script), 0o700))

	return path
}

// With --quit-if-one-screen, input that fits on one screen and is there for the
// taking never reaches the alternate screen at all. Switching there and right
// back again makes the terminal flash.
//
// Ref: https://github.com/walles/moor/issues/425
func TestQuitIfOneScreenNeverEntersAlternateScreen(t *testing.T) {
	for _, answerBackgroundQuery := range []bool{true, false} {
		for _, fromPipe := range []bool{false, true} {
			name := fmt.Sprintf("answerBackgroundQuery=%t/fromPipe=%t", answerBackgroundQuery, fromPipe)
			t.Run(name, func(t *testing.T) {
				// Two lines fit on our 24 line screen, so moor quits by itself
				options := moorOptions{
					answerBackgroundQuery: answerBackgroundQuery,
					args:                  []string{"--quit-if-one-screen"},
				}
				if fromPipe {
					options.stdin = ptr.To("hello world 1\nhello world 2\n")
				} else {
					options.args = append(options.args, createTextFile(t, 2))
				}

				session := startMoor(t, options)
				session.wait(t)

				captured := session.captured()

				// Without this a moor that printed nothing at all would pass
				assert.Assert(t, strings.Contains(captured, "hello world 2"),
					"Never printed the contents:\n%s", humanizeEscapes(captured))

				// The leave matters as much as the enter: DECRST 1049 also
				// restores a saved cursor position, so an unpaired one can
				// teleport the user's cursor.
				assert.Assert(t, !strings.Contains(captured, altScreenEnter),
					"Entered the alternate screen:\n%s", humanizeEscapes(captured))
				assert.Assert(t, !strings.Contains(captured, altScreenLeave),
					"Left the alternate screen:\n%s", humanizeEscapes(captured))
			})
		}
	}
}

// Input that gets highlighted has to stay off the alternate screen too.
//
// Highlighting can turn contents that fit into contents that don't, so moor
// cannot know whether it is quitting until highlighting is done. It has to wait
// without painting: `MOOR=--quit-if-one-screen` plus a source file is a
// mainstream invocation, and it flashed for as long as issue #425 was open.
func TestQuitIfOneScreenNeverEntersAlternateScreenWhenHighlighted(t *testing.T) {
	for _, answerBackgroundQuery := range []bool{true, false} {
		for _, fromPipe := range []bool{false, true} {
			name := fmt.Sprintf("answerBackgroundQuery=%t/fromPipe=%t", answerBackgroundQuery, fromPipe)
			t.Run(name, func(t *testing.T) {
				// One identifier, because highlighting would put escape
				// sequences inside a phrase
				marker := "hello_world"

				// Enough lines that highlighting is still going when the pager
				// starts, few enough that they fit on our 24 line screen
				lineCount := 20

				options := moorOptions{
					answerBackgroundQuery: answerBackgroundQuery,
					args:                  []string{"--quit-if-one-screen"},
				}
				if fromPipe {
					options.stdin = ptr.To(jsonLines(lineCount, marker))
				} else {
					options.args = append(options.args, createSourceFile(t, lineCount, marker))
				}

				session := startMoor(t, options)
				session.wait(t)

				captured := session.captured()

				// Without this a moor that printed nothing at all would pass
				assert.Assert(t, strings.Contains(captured, marker),
					"Never printed the contents:\n%s", humanizeEscapes(captured))

				assert.Assert(t, !strings.Contains(captured, altScreenEnter),
					"Entered the alternate screen:\n%s", humanizeEscapes(captured))
				assert.Assert(t, !strings.Contains(captured, altScreenLeave),
					"Left the alternate screen:\n%s", humanizeEscapes(captured))
			})
		}
	}
}
