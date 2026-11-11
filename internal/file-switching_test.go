package internal

import (
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
