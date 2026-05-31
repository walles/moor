package internal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

const screenHeight = 60

// Repro for: https://github.com/walles/moor/issues/166
func testCanonicalize1000(t *testing.T, withStatusBar bool, currentStartLine linemetadata.Index, lastVisibleLine linemetadata.Index) {
	pager := Pager{}
	pager.screen = twin.NewFakeScreen(100, screenHeight)
	pager.readers = []*reader.ReaderImpl{reader.NewFromTextForTesting("test", strings.Repeat("a\n", 2000))}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}
	pager.ShowLineNumbers = true
	pager.showLineNumbers = true
	pager.ShowStatusBar = withStatusBar
	pager.scrollPosition = scrollPosition{
		internalDontTouch: scrollPositionInternal{
			lineIndex:      &currentStartLine,
			delta:          0,
			name:           "findFirstHit",
			canonicalizing: false,
		},
	}

	lastVisiblePosition := scrollPosition{
		internalDontTouch: scrollPositionInternal{
			lineIndex: &lastVisibleLine,
			delta:     0,
			name:      "Last Visible Position",
		},
	}

	assert.Equal(t, *lastVisiblePosition.lineIndex(&pager), lastVisibleLine)
}

func TestCanonicalize1000WithStatusBar(t *testing.T) {
	for startLine := 0; startLine < 1500; startLine++ {
		t.Run(fmt.Sprint("startLine=", startLine), func(t *testing.T) {
			testCanonicalize1000(t, true,
				linemetadata.IndexFromZeroBased(startLine),
				linemetadata.IndexFromZeroBased(startLine+screenHeight-2),
			)
		})
	}
}

func TestCanonicalize1000WithoutStatusBar(t *testing.T) {
	for startLine := 0; startLine < 1500; startLine++ {
		t.Run(fmt.Sprint("startLine=", startLine), func(t *testing.T) {
			testCanonicalize1000(t, true,
				linemetadata.IndexFromZeroBased(startLine),
				linemetadata.IndexFromZeroBased(startLine+screenHeight-1),
			)
		})
	}
}

// Try scrolling between two points, on a 80 x screenHeight screen with 1492
// lines of input.
func tryScrollAmount(t *testing.T, scrollFrom linemetadata.Index, scrollDistance linemetadata.ScreenLines) {
	// Create 1492 lines of single-char content
	pager := Pager{}
	pager.screen = twin.NewFakeScreen(80, screenHeight)
	pager.readers = []*reader.ReaderImpl{reader.NewFromTextForTesting("test", strings.Repeat("x\n", 1492))}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}
	pager.ShowLineNumbers = true
	pager.showLineNumbers = true

	pager.scrollPosition = scrollPosition{
		internalDontTouch: scrollPositionInternal{
			name:      "tryScrollAmount",
			lineIndex: &scrollFrom,
			delta:     scrollDistance,
		},
	}

	// Trigger rendering (and canonicalization). If the prefix is miscomputed
	// this would previously panic inside createLinePrefix().
	rendered := pager.renderLines()

	// Sanity check the result
	assert.Assert(t, rendered.lines != nil)
	assert.Equal(t, len(rendered.lines), int(pager.visibleHeight()))
	assert.Equal(t, rendered.lines[0].inputLineIndex, scrollFrom.NonWrappingAdd(int(scrollDistance)))
}

// Repro for https://github.com/walles/moor/issues/313: Rapid scroll
// (deltaScreenLines > 0) crossing from 3 to 4 digits must not panic due to
// too-short number prefix length.
func TestFastScrollAcross1000DoesNotPanic(t *testing.T) {
	tryScrollAmount(t, linemetadata.IndexFromZeroBased(900), 200)
}

// Repro for https://github.com/walles/moor/issues/338
func TestIssue338(t *testing.T) {
	tryScrollAmount(t, linemetadata.IndexFromZeroBased(1000), -60)
}

func TestMultipleScrollStartsAcross1000DoNotPanic(t *testing.T) {
	for scrollFrom := 1000 - screenHeight - 10; scrollFrom <= 1000; scrollFrom++ {
		tryScrollAmount(t, linemetadata.IndexFromZeroBased(scrollFrom), linemetadata.ScreenLines(screenHeight))
	}
}

func TestMultipleScrollDistancesAcross1000DoNotPanic(t *testing.T) {
	scrollFrom := 1000 - screenHeight - 10
	for scrollDistance := 0; scrollDistance <= 3*screenHeight; scrollDistance++ {
		tryScrollAmount(t, linemetadata.IndexFromZeroBased(scrollFrom), linemetadata.ScreenLines(scrollDistance))
	}
}

func TestMultipleBackwardsScrollStartsAcross1000DoNotPanic(t *testing.T) {
	for scrollFrom := 1000 + screenHeight + 10; scrollFrom >= 1000; scrollFrom-- {
		tryScrollAmount(t, linemetadata.IndexFromZeroBased(scrollFrom), linemetadata.ScreenLines(-screenHeight))
	}
}

func TestMultipleBackwardsScrollDistancesAcross1000DoNotPanic(t *testing.T) {
	scrollFrom := 1000 + screenHeight + 10
	for scrollDistance := 0; scrollDistance <= 3*screenHeight; scrollDistance++ {
		tryScrollAmount(t, linemetadata.IndexFromZeroBased(scrollFrom), linemetadata.ScreenLines(-scrollDistance))
	}
}

// Repro for https://github.com/walles/moor/issues/399.
//
// The bug relied on `canonicalize()` previously using a wider line-number
// prefix length than `internalRenderLines()`, due to `canonicalize()` looking
// ahead by `deltaScreenLines`. For example, if line 900 has a delta of 569, it
// would check ahead ~600 lines, reaching index 1500 (4 digits), assuming a
// 4-digit line number gutter length. But `internalRenderLines()` only looks at
// visible lines (e.g. index 988), which is 3 digits, and therefore would
// reserve a smaller gutter length.
//
// Because of this difference, `canonicalize()` used to leave less horizontal
// space for text than `internalRenderLines()` did. This test triggers the edge
// case by creating a line length that wraps exactly 570 times underneath the
// wider gutter (4 digits -> forces more wraps), but only 569 times underneath
// the narrower gutter (3 digits -> wider space -> fewer wraps). When the UI
// tried to display the 569th wrap, `internalRenderLines()` had not generated
// it, resulting in a "not found in allLines" panic!
func TestIssue399(t *testing.T) {
	// A line of 102601 characters wraps 570 times on a 185-width screen with a
	// 4-digit gutter, but only 569 times with a 3-digit gutter.
	const magicBug399LineLength = 102601

	lineCount := 2000
	var lines []string
	for i := range lineCount {
		if i == 900 {
			lines = append(lines, strings.Repeat("A", magicBug399LineLength))
		} else {
			lines = append(lines, "A short line")
		}
	}
	txt := strings.Join(lines, "\n")
	r := reader.NewFromTextForTesting("test", txt)

	pager := NewPager(r)

	spci := scrollPositionInternal{
		lineIndex: func(i int) *linemetadata.Index {
			idx := linemetadata.IndexFromZeroBased(i)
			return &idx
		}(900),
		delta: 569,
		name:  "scrollToSearchHits",
	}

	pager.scrollPosition = scrollPosition{
		internalDontTouch: spci,
	}

	pager.WrapLongLines = true
	pager.ShowLineNumbers = true
	pager.screen = twin.NewFakeScreen(185, 88)

	pager.redraw("")
}

// A mock reader that mimics a concurrent filtering out of all lines.
// It returns >0 lines initially, but then returns 0 lines later.
type shrinkingReader struct {
	reader.Reader
	calls int
}

func (s *shrinkingReader) GetLineCount() int {
	s.calls++
	// Simulate a file that initially has lines, so the pager begins to process them...
	if s.calls <= 2 {
		return 10
	}
	// ...but is then truncated concurrently down to 0 lines.
	return 0
}

func (s *shrinkingReader) GetLine(index linemetadata.Index) *reader.NumberedLine {
	return nil
}

func TestIssue415Panic(t *testing.T) {
	r := reader.NewFromTextForTesting("test", "A\nB\nC\nD\nE\nF\nG\nH\nI\n")

	pager := NewPager(r)
	pager.filteringReader.BackingReader = &shrinkingReader{Reader: r}
	spci := scrollPositionInternal{
		lineIndex: func(i int) *linemetadata.Index {
			idx := linemetadata.IndexFromZeroBased(i)
			return &idx
		}(10), // start out of bounds (past what the reader claims it has)
		delta: 0,
		name:  "test415",
	}

	pager.scrollPosition = scrollPosition{
		internalDontTouch: spci,
	}

	pager.WrapLongLines = true
	pager.ShowLineNumbers = true
	pager.screen = twin.NewFakeScreen(80, 24)

	pager.redraw("")
}

// Regression test for https://github.com/walles/moor/issues/426
func TestIssue426(t *testing.T) {
	// Create a small file with only 10 lines
	smallContent := strings.Repeat("short\n", 10)
	smallReader := reader.NewFromTextForTesting("small", smallContent)

	assert.NilError(t, smallReader.Wait())

	pager := Pager{
		screen:         twin.NewFakeScreen(20, 10),
		readers:        []*reader.ReaderImpl{smallReader},
		currentReader:  0,
		readerSwitched: make(chan struct{}, 1),
	}

	pager.mode = PagerModeViewing{&pager}

	pager.filteringReader = FilteringReader{
		BackingReader: smallReader,
		Filter:        &pager.filter,
	}

	pager.scrollPosition = newScrollPosition("TestIssue426")

	// Manually set an out-of-bounds scroll position (70)
	seventy := linemetadata.IndexFromZeroBased(70)
	pager.scrollPosition.internalDontTouch.lineIndex = &seventy
	pager.scrollPosition.internalDontTouch.delta = 0

	// Set the canonical state to match the current pager state. This tricks
	// canonicalize() into thinking the position is already valid so it won't
	// clamp the lineIndex, even though 70 > file's line count (10).
	pager.scrollPosition.internalDontTouch.canonical = scrollPositionCanonical{
		width:            20,
		height:           10,
		pagerLineCount:   10,
		lineIndex:        &seventy,
		deltaScreenLines: 0,
	}

	// This used to panic when the scroll position's lineIndex was out of bounds
	// for the current file, but the canonical state was stale, causing
	// canonicalize() to skip clamping.
	pager.internalRenderLines(true)
}
