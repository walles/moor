package internal

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/twin"
	"gotest.tools/v3/assert"
)

func TestScrollMaxRight_AllLinesFitWithLineNumbers(t *testing.T) {
	// Case 2: All lines fit with line numbers
	screenWidth := 20
	widestLineWidth := 16 // Just below available width
	line := strings.Repeat("x", widestLineWidth)
	reader := reader.NewFromTextForTesting("test", line)
	screen := twin.NewFakeScreen(screenWidth, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.WrapLongLines = false

	pager.scrollMaxRight()
	assert.Equal(t, 0, pager.leftColumnZeroBased)
	assert.Equal(t, true, pager.showLineNumbers)
}

func TestScrollMaxRight_AllLinesFitWithoutLineNumbers1(t *testing.T) {
	// Case 2: All lines fit with line numbers
	screenWidth := 20
	widestLineWidth := 17 // Just above available width with line numbers
	line := strings.Repeat("x", widestLineWidth)
	reader := reader.NewFromTextForTesting("test", line)
	screen := twin.NewFakeScreen(screenWidth, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.WrapLongLines = false

	pager.scrollMaxRight()
	assert.Equal(t, 0, pager.leftColumnZeroBased)
	assert.Equal(t, false, pager.showLineNumbers)
}

func TestScrollMaxRight_AllLinesFitWithoutLineNumbers2(t *testing.T) {
	// Case 3: All lines fit only if line numbers are hidden, just at the edge
	screenWidth := 20
	widestLineWidth := 20 // Above available with line numbers, just below without
	line := strings.Repeat("x", widestLineWidth)
	reader := reader.NewFromTextForTesting("test", line)
	screen := twin.NewFakeScreen(screenWidth, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.WrapLongLines = false

	pager.scrollMaxRight()
	assert.Equal(t, 0, pager.leftColumnZeroBased)
	assert.Equal(t, false, pager.showLineNumbers)
}

func TestScrollMaxRight_WidestLineExceedsScreenWidth_Edge(t *testing.T) {
	// Case 4: Widest line just exceeds available width even without line numbers
	screenWidth := 20
	widestLineWidth := 21 // Just above available width without line numbers
	line := strings.Repeat("x", widestLineWidth)
	reader := reader.NewFromTextForTesting("test", line)
	screen := twin.NewFakeScreen(screenWidth, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.WrapLongLines = false

	pager.scrollMaxRight()
	assert.Equal(t, 1, pager.leftColumnZeroBased)
	assert.Equal(t, false, pager.showLineNumbers)
}

func modeName(pager *Pager) string {
	switch pager.mode.(type) {
	case PagerModeViewing:
		return "Viewing"
	case PagerModeNotFound:
		return "NotFound"
	case *PagerModeSearch:
		return "Search"
	case *PagerModeGotoLine:
		return "GotoLine"
	default:
		panic("Unknown pager mode")
	}
}

// Create a pager with three screen lines reading from a six lines stream
func createThreeLinesPager(t *testing.T) *Pager {
	reader := reader.NewFromTextForTesting("", "a\nb\nc\nd\ne\nf\n")

	screen := twin.NewFakeScreen(20, 3)
	pager := NewPager(reader)

	pager.screen = screen

	assert.Equal(t, "Viewing", modeName(pager), "Initial pager state")

	return pager
}

func TestScrollToNextSearchHit_StartAtBottom(t *testing.T) {
	// Create a pager scrolled to the last line
	pager := createThreeLinesPager(t)
	pager.scrollToEnd()

	// Set the search to something that doesn't exist in this pager
	pager.search.For("xxx")

	// Scroll to the next search hit
	pager.scrollToNextSearchHit(false)

	assert.Equal(t, "NotFound", modeName(pager))
}

func TestScrollToNextSearchHit_StartAtTop(t *testing.T) {
	// Create a pager scrolled to the first line
	pager := createThreeLinesPager(t)

	// Set the search to something that doesn't exist in this pager
	pager.search.For("xxx")

	// Scroll to the next search hit
	pager.scrollToNextSearchHit(false)

	assert.Equal(t, "NotFound", modeName(pager))
}

func TestScrollToNextSearchHit_WrapAfterNotFound(t *testing.T) {
	// Create a pager scrolled to the last line
	pager := createThreeLinesPager(t)
	pager.scrollToEnd()

	// Search for "a", it's on the first line (ref createThreeLinesPager())
	pager.search.For("a")

	// Scroll to the next search hit, this should take us into _NotFound
	pager.scrollToNextSearchHit(false)
	assert.Equal(t, "NotFound", modeName(pager))

	// Scroll to the next search hit, this should wrap the search and take us to
	// the top
	pager.scrollToNextSearchHit(true)
	assert.Equal(t, "Viewing", modeName(pager))
	assert.Assert(t, pager.lineIndex().IsZero())
}

func TestScrollToNextSearchHit_WrapAfterFound(t *testing.T) {
	// Create a pager scrolled to the last line
	pager := createThreeLinesPager(t)
	pager.scrollToEnd()

	// Search for "f", it's on the last line (ref createThreeLinesPager())
	pager.search.For("f")

	// Scroll to the next search hit, this should take us into _NotFound
	pager.scrollToNextSearchHit(false)
	assert.Equal(t, "NotFound", modeName(pager))

	// Scroll to the next search hit, this should wrap the search and take us
	// back to the bottom again
	pager.scrollToNextSearchHit(true)
	assert.Equal(t, "Viewing", modeName(pager))
	assert.Equal(t, 4, pager.lineIndex().Index())
}

// Ref: https://github.com/walles/moor/issues/152
func Test152(t *testing.T) {
	// Show a pager on a five lines terminal
	reader := reader.NewFromTextForTesting("", "a\nab\nabc\nabcd\nabcde\nabcdef\n")
	screen := twin.NewFakeScreen(20, 5)
	pager := NewPager(reader)
	pager.screen = screen
	assert.Equal(t, "Viewing", modeName(pager), "Initial pager state")

	searchMode := NewPagerModeSearch(pager, SearchDirectionForward, pager.scrollPosition)
	pager.mode = searchMode
	// Search for the first not-visible hit
	searchMode.inputBox.setText("abcde")

	assert.Equal(t, "Search", modeName(pager))
	assert.Equal(t, 2, pager.lineIndex().Index())
}

func TestScrollLeftToSearchHits_NoLineNumbers(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "a234567890")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.search.For("a")
	pager.leftColumnZeroBased = 1

	assert.Equal(t, true, pager.scrollLeftToSearchHits())
	assert.Equal(t, 0, pager.leftColumnZeroBased)
	assert.Equal(t, false, pager.showLineNumbers)
}

func TestScrollLeftToSearchHits_WithLineNumbers(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "a234567890")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.showLineNumbers = false
	pager.search.For("a")
	pager.leftColumnZeroBased = 1

	assert.Equal(t, true, pager.scrollLeftToSearchHits())
	assert.Equal(t, 0, pager.leftColumnZeroBased)
	assert.Equal(t, true, pager.showLineNumbers)
}

func TestScrollLeftToSearchHits_JumpsDirectlyToHit(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "01234567890a234567890123456789")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.showLineNumbers = false
	pager.search.For("a")
	pager.leftColumnZeroBased = 20

	assert.Equal(t, true, pager.scrollLeftToSearchHits())
	assert.Equal(t, 6, pager.leftColumnZeroBased,
		"We started at 20. The search hit 'a' is at column 11. Screen width 10. The start column is placed so 'a' is centered: 11 - 10 / 2 = 6")
	assert.Equal(t, false, pager.showLineNumbers)
}

// If the screen is too narrow for line numbers, there's no point in scrolling.
// This test has provoked some panics.
func TestScrollRightToSearchHits_NarrowScreen(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "abcdefg")
	screen := twin.NewFakeScreen(1, 5)
	pager := NewPager(reader)
	pager.screen = screen

	pager.showLineNumbers = false
	assert.Equal(t, pager.scrollRightToSearchHits(), false)

	pager.showLineNumbers = true
	assert.Equal(t, pager.scrollRightToSearchHits(), false)
}

func TestScrollRightToSearchHits_DisableLineNumbersToSeeHit0(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "12345a")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.showLineNumbers = true
	pager.search.For("a")
	pager.leftColumnZeroBased = 0

	assert.Equal(t, true, pager.scrollRightToSearchHits())
	assert.Equal(t, 0, pager.leftColumnZeroBased, "Should scroll right to bring 'a' into view")
	assert.Equal(t, false, pager.showLineNumbers, "Should disable line numbers to fit search hit")
}

func TestScrollRightToSearchHits_DisableLineNumbersToSeeHit(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "123456789a")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = true
	pager.showLineNumbers = true
	pager.search.For("a")
	pager.leftColumnZeroBased = 0

	assert.Equal(t, true, pager.scrollRightToSearchHits())
	assert.Equal(t, 0, pager.leftColumnZeroBased, "Should scroll right to bring 'a' into view")
	assert.Equal(t, false, pager.showLineNumbers, "Should disable line numbers to fit search hit")
}

func TestScrollRightToSearchHits_HiddenByScrollMarker(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "123456789a234567890")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.search.For("a")
	pager.leftColumnZeroBased = 0

	assert.Equal(t, true, pager.scrollRightToSearchHits())
	assert.Equal(t, 4, pager.leftColumnZeroBased, "Should scroll right to bring 'a' into view and center it")
}

// Repro case for https://github.com/walles/moor/issues/337.
func TestScrollRightToSearchHits_Issue337(t *testing.T) {
	reader := reader.NewFromTextForTesting("", "123456a89012345")
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.search.For("a")
	pager.leftColumnZeroBased = 0

	assert.Equal(t, false, pager.scrollRightToSearchHits(), "Search hit was already visible, should not have scrolled")
	assert.Equal(t, 0, pager.leftColumnZeroBased, "Should not have scrolled")
}

func TestScrollRightToSearchHits_LastCharHit(t *testing.T) {
	const line = "x0123456789a"
	reader := reader.NewFromTextForTesting("", line)
	screen := twin.NewFakeScreen(10, 5)
	pager := NewPager(reader)
	pager.screen = screen
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.search.For("a")
	pager.leftColumnZeroBased = 0

	assert.Equal(t, true, pager.scrollRightToSearchHits())

	// Search hit at col 7 (considering prefix offsets) should be centered.
	// If 'a' is at index 11: 11 - 10/2 = 6, adjusted to 2 by max width bounds.
	assert.Equal(t, 2, pager.leftColumnZeroBased, "hit should be right-bounded")
}

func TestScrollRightToSearchHits_OnlyStartOfHitTriggers(t *testing.T) {
	// Arrange: create a line with a multi-rune search hit
	line := "abcDEFGHIJKLMNOPQRSTUVWXYZ"
	readerImpl := reader.NewFromTextForTesting("test", line)
	screen := twin.NewFakeScreen(5, 2) // Narrow screen to force scrolling
	pager := NewPager(readerImpl)
	pager.search.For("DEFGHIJ") // Match starts at index 3
	pager.screen = screen
	pager.WrapLongLines = false
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	assert.Assert(t, !pager.scrollRightToSearchHits(), "No more search hit starts to the right, should not scroll")
}

// Ref: https://github.com/walles/moor/pull/414
func BenchmarkScrollRightToSearchHits(b *testing.B) {
	log.SetLevel(log.WarnLevel) // Stop info logs from polluting benchmark output

	// Create a ~100kb line with the search hit near the end
	text := strings.Repeat("a", 10_000) + "gunzip" + strings.Repeat("b", 100)
	testReader := reader.NewFromTextForTesting("BenchmarkScrollRightToSearchHits", text)

	// Create a screen of 180 chars wide as mentioned in the PR comment
	screen := twin.NewFakeScreen(180, 50)

	for b.Loop() {
		// Pause the timer while we reset the pager state for the next iteration
		b.StopTimer()
		pager := NewPager(testReader)
		pager.screen = screen
		pager.ShowLineNumbers = false
		pager.showLineNumbers = false
		pager.leftColumnZeroBased = 0

		// Initiate the search
		pager.search.For("gunzip")
		b.StartTimer()

		// This loop evaluates the performance bottleneck described in PR #414
		pager.scrollRightToSearchHits()
	}
}

// Ref: https://github.com/walles/moor/pull/414
func BenchmarkScrollLeftToSearchHits(b *testing.B) {
	log.SetLevel(log.WarnLevel) // Stop info logs from polluting benchmark output

	// Create a ~100kb line with the search hit near the beginning
	text := strings.Repeat("b", 100) + "gunzip" + strings.Repeat("a", 10_000)
	testReader := reader.NewFromTextForTesting("BenchmarkScrollLeftToSearchHits", text)

	// Create a screen of 180 chars wide as mentioned in the PR comment
	screen := twin.NewFakeScreen(180, 50)

	for b.Loop() {
		// Pause the timer while we reset the pager state for the next iteration
		b.StopTimer()
		pager := NewPager(testReader)
		pager.screen = screen
		pager.ShowLineNumbers = false
		pager.showLineNumbers = false
		pager.leftColumnZeroBased = 10_000 // Start way to the right so we can scroll left

		// Initiate the search
		pager.search.For("gunzip")
		b.StartTimer()

		pager.scrollLeftToSearchHits()
	}
}

func TestScrollRightToSearchHits_WideRunes(t *testing.T) {
	// Chinese characters are typically printed 2 visual columns wide, but take 1 rune.
	// 10 runes = 20 visual columns.
	// The "HIT" is at rune index 10, but visual column 20.
	line := strings.Repeat("世", 10) + "HIT"
	readerImpl := reader.NewFromTextForTesting("test", line)

	// Since the screen is 5 wide, the pager must correctly account for the
	// visual width of the characters (not just rune index) to ensure it scrolls
	// far enough for "HIT" (at column 20) to become visible.
	screen := twin.NewFakeScreen(5, 5)
	pager := NewPager(readerImpl)
	pager.search.For("HIT")
	pager.screen = screen
	pager.WrapLongLines = false
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false

	scrolled := pager.scrollRightToSearchHits()
	assert.Assert(t, scrolled, "Should have scrolled right")
	assert.Equal(t, true, pager.searchHitIsVisible(), "The hit should be visible")
}

func TestScrollLeftToSearchHits_WideRunes(t *testing.T) {
	// Chinese characters are typically printed 2 visual columns wide, but take 1 rune.
	// 10 runes = 20 visual columns.
	// The "HIT" is at rune index 10, but visual column 20.
	line := strings.Repeat("世", 10) + "HIT"
	readerImpl := reader.NewFromTextForTesting("test", line)

	// Since the screen is 5 wide, the pager must correctly account for the
	// visual width of the characters (not just rune index) to ensure it scrolls
	// far enough (or stops at the right place) for "HIT" (at column 20) to become visible.
	screen := twin.NewFakeScreen(5, 5)
	pager := NewPager(readerImpl)
	pager.search.For("HIT")
	pager.screen = screen
	pager.WrapLongLines = false
	pager.ShowLineNumbers = false
	pager.showLineNumbers = false
	pager.leftColumnZeroBased = 30 // Start scrolled far to the right, past the hit

	scrolled := pager.scrollLeftToSearchHits()
	assert.Assert(t, scrolled, "Should have scrolled left")
	assert.Equal(t, 18, pager.leftColumnZeroBased, "The hit starts at visual column 20. Screen width is 5. Target is centered: 20 - 5 / 2 = 18")
	assert.Equal(t, true, pager.searchHitIsVisible(), "The hit should be visible")
}
