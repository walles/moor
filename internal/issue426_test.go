package internal

import (
	"strings"
	"testing"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/twin"
)

// TestIssue426 reproduces the panic from https://github.com/walles/moor/issues/426
//
// The panic occurs when the scroll position's lineIndex is out of bounds for the
// current file, but the canonical state is stale (matches the old file state),
// so canonicalize() doesn't clamp it, leading to a mismatch between lineIndexToShow
// and the actual scroll position used in the rendering loop.
func TestIssue426(t *testing.T) {
	// Create a small file with only 10 lines
	smallContent := strings.Repeat("short\n", 10)
	smallR := reader.NewFromTextForTesting("small", smallContent)
	smallR.Wait()

	pager := Pager{
		screen:         twin.NewFakeScreen(20, 10),
		readers:        []*reader.ReaderImpl{smallR},
		currentReader:  0,
		readerSwitched: make(chan struct{}, 1),
	}

	pager.mode = PagerModeViewing{&pager}
	pager.ShowStatusBar = false
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	pager.filteringReader = FilteringReader{
		BackingReader: smallR,
		Filter:        &pager.filter,
	}

	pager.scrollPosition = newScrollPosition("TestIssue426")

	// Manually set an out-of-bounds scroll position (70)
	seventy := linemetadata.IndexFromZeroBased(70)
	pager.scrollPosition.internalDontTouch.lineIndex = &seventy
	pager.scrollPosition.internalDontTouch.delta = 0

	// Set the canonical state to match the current pager state
	// This tricks canonicalize() into thinking the position is already valid
	// so it won't clamp the lineIndex, even though 70 > file's line count (10)
	pager.scrollPosition.internalDontTouch.canonical = scrollPositionCanonical{
		width:            20,
		height:           10,
		showLineNumbers:  false,
		showStatusBar:    false,
		wrapLongLines:    false,
		pagerLineCount:   10,
		lineIndex:        &seventy,
		deltaScreenLines: 0,
	}

	// This should panic with: scrollPosition index={70} not found in allLines
	// because:
	// 1. lineIndexToShow = 70 (from p.lineIndex(), canonicalize returns immediately)
	// 2. GetLines(70, 10) on 10-line file clamps to (0, 9), returns lines 0-9
	// 3. Loop: p.lineIndex() returns 70 (canonicalize still returns immediately)
	// 4. Loop looks for inputLineIndex == 70 in allLines (0-9), doesn't find it
	pager.internalRenderLines(true)
}
