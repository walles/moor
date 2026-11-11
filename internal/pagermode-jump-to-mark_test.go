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

func newMarksTestPager(t *testing.T, readers ...*reader.ReaderImpl) *Pager {
	t.Helper()

	for _, r := range readers {
		assert.NilError(t, r.Wait())
	}

	pager := NewPager(readers...)
	pager.screen = twin.NewFakeScreen(20, 10)
	pager.bookmarks = make(map[rune]scrollPosition)
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
