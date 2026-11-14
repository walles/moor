package internal

import (
	"strings"
	"testing"

	"github.com/walles/twin"
	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
)

func newMarksTestPager(t *testing.T, readers ...*reader.ReaderImpl) *Pager {
	t.Helper()

	for _, r := range readers {
		assert.NilError(t, r.Wait())
	}

	pager := NewPager(readers...)
	pager.screen = twin.NewFakeScreen(20, 10)
	return pager
}

// Opening the "'" prompt and then cancelling it should not stop following.
func TestCancellingJumpToMarkPromptKeepsFollowing(t *testing.T) {
	pager := newMarksTestPager(t, reader.NewFromTextForTesting("a", "a\n"))
	pager.setTargetLine(ptr.To(linemetadata.IndexMax()))

	pager.mode.onRune('\'')
	pager.mode.onKey(twin.KeyEscape)

	assert.Assert(t, pager.isViewing())
	assert.Assert(t, pager.TargetLine != nil)
	assert.Equal(t, *pager.TargetLine, linemetadata.IndexMax())
}

// Jumping to a mark while following should stop following, otherwise we would
// just be pulled back to the end.
func TestJumpingToMarkStopsFollowing(t *testing.T) {
	pager := newMarksTestPager(t, reader.NewFromTextForTesting("a", strings.Repeat("a\n", 100)))

	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(10), "test")
	pager.mode.onRune('m')
	pager.mode.onRune('x')

	pager.setTargetLine(ptr.To(linemetadata.IndexMax()))
	pager.mode.onRune('\'')
	pager.mode.onRune('x')

	assert.Assert(t, pager.isViewing())
	assert.Assert(t, pager.TargetLine == nil)
	assert.Equal(t, pager.lineIndex().Index(), 10)
}

func setMark(pager *Pager, topLineZeroBased int, mark rune) {
	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(topLineZeroBased), "test")
	pager.mode.onRune('m')
	pager.mode.onRune(mark)
}

func jumpToMark(pager *Pager, mark rune) {
	pager.mode.onRune('\'')
	pager.mode.onRune(mark)
}

func TestJumpToMarkInOtherFile(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.nextFile()
	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(50), "test")

	jumpToMark(pager, 'x')
	assert.Equal(t, pager.currentView, 0)
	assert.Equal(t, firstRenderedLine(t, pager), "a100")
	assert.Assert(t, pager.isViewing())

	// Switching back to the other file should take us to where we were there
	pager.nextFile()
	assert.Equal(t, firstRenderedLine(t, pager), "b50")
}

// Marks are global, so setting x in one file should replace x in another
func TestSettingMarkInOtherFileReplacesIt(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.nextFile()
	setMark(pager, 50, 'x')

	pager.previousFile()
	jumpToMark(pager, 'x')
	assert.Equal(t, pager.currentView, 1)
	assert.Equal(t, firstRenderedLine(t, pager), "b50")
}

func TestMarkSurvivesAddingFilter(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.filter.For("a1")
	jumpToMark(pager, 'x')
	assert.Equal(t, firstRenderedLine(t, pager), "a100")
	assert.Assert(t, pager.isViewing())
}

func TestMarkSurvivesRemovingFilter(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	pager.filter.For("a2")

	// Filtered line 1 is a20, after a2
	setMark(pager, 1, 'x')

	pager.filter = search.Search{}
	jumpToMark(pager, 'x')
	assert.Equal(t, firstRenderedLine(t, pager), "a20")
	assert.Assert(t, pager.isViewing())
}

// If the marked line is hidden by the filter, show the closest line above it
// at the top. Right below that is where the marked line would have been.
func TestJumpToMarkHiddenByFilter(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.filter.For("a2")
	jumpToMark(pager, 'x')
	assert.Equal(t, firstRenderedLine(t, pager), "a29")

	info, isInfo := pager.mode.(*PagerModeInfo)
	assert.Assert(t, isInfo, "Expected an info message, got %T", pager.mode)
	assert.Equal(t, info.Text, "Mark x is hidden by the filter")
}

// If the marked line is hidden by the filter, and there are no lines above it,
// go to the top.
func TestJumpToMarkHiddenByFilterNothingAbove(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 1, 'x')

	pager.filter.For("a2")
	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(50), "test")
	jumpToMark(pager, 'x')
	assert.Equal(t, firstRenderedLine(t, pager), "a2")

	info, isInfo := pager.mode.(*PagerModeInfo)
	assert.Assert(t, isInfo, "Expected an info message, got %T", pager.mode)
	assert.Equal(t, info.Text, "Mark x is hidden by the filter")
}

// Jumping to a mark from help should leave help and go to the mark
func TestJumpToMarkFromHelp(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.nextFile()
	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(50), "test")

	pager.mode.onRune('h')
	assert.Assert(t, pager.isShowingHelp)

	jumpToMark(pager, 'x')
	assert.Assert(t, !pager.isShowingHelp)
	assert.Assert(t, pager.isViewing())
	assert.Equal(t, pager.currentView, 0)
	assert.Equal(t, firstRenderedLine(t, pager), "a100")

	// The other file should still be where it was before help
	pager.nextFile()
	assert.Equal(t, firstRenderedLine(t, pager), "b50")
}

// Leaving help is only worth it if we're going somewhere
func TestJumpToMissingMarkFromHelpStaysInHelp(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.mode.onRune('h')
	jumpToMark(pager, 'y')
	assert.Assert(t, pager.isShowingHelp)

	info, isInfo := pager.mode.(*PagerModeInfo)
	assert.Assert(t, isInfo, "Expected an info message, got %T", pager.mode)
	assert.Equal(t, info.Text, "Mark y is not set")
}

// With no marks set, the jump prompt offers to set one. Not from help though,
// there is no file line to mark there.
func TestNoSettingMarksFromHelpJumpPrompt(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.mode.onRune('h')
	pager.mode.onRune('\'')

	jumpMode, isJumping := pager.mode.(PagerModeJumpToMark)
	assert.Assert(t, isJumping, "Expected the jump prompt, got %T", pager.mode)
	assert.Equal(t, jumpMode.getMarkPrompt(), "No marks set")

	pager.mode.onRune('m')
	assert.Assert(t, pager.isShowingHelp)
	_, isMarking := pager.mode.(PagerModeMark)
	assert.Assert(t, !isMarking)
	assert.Equal(t, len(pager.bookmarks), 0)
}

// With no marks set, there is nothing to jump to, so keys should work just like
// when viewing
func TestJumpPromptWithoutMarksPassesKeysOn(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	pager.mode.onRune('h')
	pager.mode.onRune('\'')
	pager.mode.onRune('f')

	assert.Assert(t, pager.isShowingHelp)
	assert.Assert(t, pager.isViewing(), "Expected viewing, got %T", pager.mode)
	assert.Assert(t, pager.lineIndex().Index() > 0, "Expected 'f' to scroll down")
}

func TestJumpToMissingMarkSaysSo(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')
	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(50), "test")

	jumpToMark(pager, 'y')
	assert.Equal(t, firstRenderedLine(t, pager), "a50")

	info, isInfo := pager.mode.(*PagerModeInfo)
	assert.Assert(t, isInfo, "Expected an info message, got %T", pager.mode)
	assert.Equal(t, info.Text, "Mark y is not set")
}

// Cancelling the jump prompt is not a mistake, so no message
func TestCancellingJumpToMarkSaysNothing(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	pager.mode.onRune('\'')
	pager.mode.onKey(twin.KeyEscape)
	assert.Assert(t, pager.isViewing())
}

// Setting a mark doesn't move the view, so it shouldn't stop following. The
// mark should be on the line that was at the top when 'm' was pressed.
func TestSettingMarkKeepsFollowing(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	pager.scrollPosition = NewScrollPositionFromIndex(linemetadata.IndexFromZeroBased(10), "test")

	// Following, but no new lines have arrived since we started
	pager.setTargetLine(ptr.To(linemetadata.IndexMax()))

	pager.mode.onRune('m')
	pager.mode.onRune('x')
	assert.Assert(t, pager.TargetLine != nil)
	assert.Equal(t, *pager.TargetLine, linemetadata.IndexMax())
	assert.Assert(t, pager.isScrolledToEnd())

	jumpToMark(pager, 'x')
	assert.Equal(t, firstRenderedLine(t, pager), "a10")
	assert.Assert(t, pager.TargetLine == nil)
}

func assertInfoMessage(t *testing.T, pager *Pager, expected string) {
	t.Helper()

	info, isInfo := pager.mode.(*PagerModeInfo)
	assert.Assert(t, isInfo, "Expected an info message, got %T", pager.mode)
	assert.Equal(t, info.Text, expected)
}

// Marks are invisible, so confirm that one got set
func TestSettingMarkSaysSo(t *testing.T) {
	pager := newFileSwitchingTestPager(t)

	setMark(pager, 100, 'x')
	assertInfoMessage(t, pager, "Mark x set")

	// Setting it again in the same place doesn't move it
	setMark(pager, 100, 'x')
	assertInfoMessage(t, pager, "Mark x set")
}

// Replacing a mark, possibly in another file, deserves a heads up
func TestReplacingMarkSaysSo(t *testing.T) {
	pager := newFileSwitchingTestPager(t)
	setMark(pager, 100, 'x')

	setMark(pager, 50, 'x')
	assertInfoMessage(t, pager, "Mark x moved")

	pager.nextFile()
	setMark(pager, 50, 'x')
	assertInfoMessage(t, pager, "Mark x moved")
}
