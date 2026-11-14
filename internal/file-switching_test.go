package internal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/walles/twin"
	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/reader"
)

func TestIssue426SwitchingToSmallerFileClearsStaleScrollPosition(t *testing.T) {
	largeReader := reader.NewFromTextForTesting("large", strings.Repeat("large\n", 118))
	smallReader := reader.NewFromTextForTesting("small", strings.Repeat("small\n", 48))
	assert.NilError(t, largeReader.Wait())
	assert.NilError(t, smallReader.Wait())

	pager := NewPager(largeReader, smallReader)
	pager.screen = twin.NewFakeScreen(190, 49)
	pager.WrapLongLines = true
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.filteringReader.SetBackingReader(smallReader)

	// This matches the stale post-switch state from issue #426: the reader has
	// moved to a shorter file, but the scroll position still points below it.
	pager.scrollPosition = scrollPosition{
		internalDontTouch: scrollPositionInternal{
			lineIndex: ptr.To(linemetadata.IndexFromZeroBased(70)),
			delta:     0,
			name:      "Issue 426 stale file switch",
		},
	}
	pager.scrollPosition.internalDontTouch.canonical = canonicalFromPager(pager)

	pager.nextFile()

	rendered := pager.renderLines()
	assert.Assert(t, len(rendered.lines) > 0)
	assert.Equal(t, renderedToString(rendered.lines[0].cells), "small")
	assert.Equal(t, pager.lineIndex().Index(), 0)
}

// Opening the ':' prompt and then cancelling it should not stop following.
func TestCancellingColonPromptKeepsFollowing(t *testing.T) {
	pager := NewPager(
		reader.NewFromTextForTesting("a", "a\n"),
		reader.NewFromTextForTesting("b", "b\n"),
	)
	pager.screen = twin.NewFakeScreen(20, 10)
	pager.setTargetLine(ptr.To(linemetadata.IndexMax()))

	pager.mode.onRune(':')
	pager.mode.onKey(twin.KeyEscape)

	assert.Assert(t, pager.isViewing())
	assert.Assert(t, pager.TargetLine != nil)
	assert.Equal(t, *pager.TargetLine, linemetadata.IndexMax())
}

func linesForTesting(prefix string, count int) string {
	var sb strings.Builder
	for i := range count {
		fmt.Fprintf(&sb, "%s%d\n", prefix, i)
	}
	return sb.String()
}

func newFileSwitchingTestPager(t *testing.T) *Pager {
	t.Helper()

	a := reader.NewFromTextForTesting("a", linesForTesting("a", 300))
	b := reader.NewFromTextForTesting("b", linesForTesting("b", 300))
	assert.NilError(t, a.Wait())
	assert.NilError(t, b.Wait())

	pager := NewPager(a, b)
	pager.screen = twin.NewFakeScreen(20, 10)
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	return pager
}

func firstRenderedLine(t *testing.T, pager *Pager) string {
	t.Helper()

	rendered := pager.renderLines()
	assert.Assert(t, len(rendered.lines) > 0)
	return strings.TrimRight(renderedToString(rendered.lines[0].cells), " ")
}

func TestSwitchingFilesRemembersPositions(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(100), "test")
	pager.nextFile()
	assert.Equal(t, firstRenderedLine(t, pager), "b0")

	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(200), "test")
	pager.previousFile()
	assert.Equal(t, firstRenderedLine(t, pager), "a100")

	pager.nextFile()
	assert.Equal(t, firstRenderedLine(t, pager), "b200")

	pager.firstFile()
	assert.Equal(t, firstRenderedLine(t, pager), "a100")
}

func TestSwitchingFilesRemembersFilters(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.filter.For("a2")
	assert.Equal(t, firstRenderedLine(t, pager), "a2")

	pager.nextFile()
	assert.Assert(t, !pager.filter.Active())
	assert.Equal(t, firstRenderedLine(t, pager), "b0")

	pager.previousFile()
	assert.Equal(t, pager.filter.String(), "a2")
	assert.Equal(t, firstRenderedLine(t, pager), "a2")
}

func TestSwitchingFilesRemembersFollowing(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.setTargetLine(ptr.To(linemetadata.IndexMax()))
	pager.scrollTowardsTargetLine()
	assert.Assert(t, pager.isScrolledToEnd())

	pager.nextFile()
	assert.Assert(t, pager.TargetLine == nil)
	assert.Equal(t, firstRenderedLine(t, pager), "b0")

	pager.previousFile()
	assert.Assert(t, pager.TargetLine != nil)
	assert.Equal(t, *pager.TargetLine, linemetadata.IndexMax())
	assert.Assert(t, pager.isScrolledToEnd())
}

func TestSwitchingFilesRemembersHorizontalScroll(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.leftColumnZeroBased = 5
	pager.nextFile()
	assert.Equal(t, pager.leftColumnZeroBased, 0)

	pager.leftColumnZeroBased = 7
	pager.previousFile()
	assert.Equal(t, pager.leftColumnZeroBased, 5)

	pager.nextFile()
	assert.Equal(t, pager.leftColumnZeroBased, 7)
}

// The help screen is shown in front of all files. "Next" and "previous" file
// are unclear from there, and there is no file line to mark.
func TestNoFileSwitchingOrSettingMarksFromHelp(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.mode.onRune('h')
	assert.Assert(t, pager.isShowingHelp)

	for _, char := range []rune{':', 'm'} {
		pager.mode.onRune(char)
		_, isViewing := pager.mode.(PagerModeViewing)
		assert.Assert(t, isViewing, "Expected '%c' to be ignored while showing help, got %T", char, pager.mode)
	}
}

// Scrolling right first hides the line numbers, so whether they are shown is
// part of the horizontal scroll position of each file
func TestSwitchingFilesRemembersLineNumbers(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	pager.ShowLineNumbers = true
	pager.showLineNumbers = true

	// Hide the line numbers in the first file
	pager.moveRight(1)
	assert.Assert(t, !pager.showLineNumbers)

	// The second file should start out with the line numbers the user asked for
	pager.nextFile()
	assert.Assert(t, pager.showLineNumbers)

	pager.previousFile()
	assert.Assert(t, !pager.showLineNumbers)

	pager.nextFile()
	assert.Assert(t, pager.showLineNumbers)
}

// Cancelling a prompt while following should take us to the end right away,
// not when the next line arrives
func TestCancellingPromptsCatchesUpWithFollowing(t *testing.T) {
	type keypress struct {
		key  twin.KeyCode
		char rune
	}
	for name, keys := range map[string][]keypress{
		"colon then ESC":       {{char: ':'}, {key: twin.KeyEscape}},
		"colon then q":         {{char: ':'}, {char: 'q'}},
		"colon then p":         {{char: ':'}, {char: 'p'}}, // Already first, no switch
		"colon then x":         {{char: ':'}, {char: 'x'}}, // Already first, no switch
		"mark jump then ESC":   {{char: '\''}, {key: twin.KeyEscape}},
		"mark jump then Enter": {{char: '\''}, {key: twin.KeyEnter}},
		"mark jump not set":    {{char: '\''}, {char: 'y'}},
		"set mark then ESC":    {{char: 'm'}, {key: twin.KeyEscape}},
	} {
		t.Run(name, func(t *testing.T) {
			pager := newFileSwitchingTestPager(t)

			// So that 'y' below is a mark that isn't set. With no marks at all,
			// 'y' would scroll up instead.
			setMark(pager, 0, 'x')

			// Following, but no new lines have arrived since we started
			pager.setTargetLine(ptr.To(linemetadata.IndexMax()))
			assert.Assert(t, !pager.isScrolledToEnd())

			for _, keypress := range keys {
				if keypress.char != 0 {
					pager.mode.onRune(keypress.char)
				} else {
					pager.mode.onKey(keypress.key)
				}
			}

			assert.Assert(t, pager.isScrolledToEnd())
		})
	}
}
