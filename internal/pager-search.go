package internal

import (
	"github.com/rivo/uniseg"
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
)

// Scroll to the next search hit, while the user is typing the search string.
func (p *Pager) scrollToSearchHits() {
	if p.search.Inactive() {
		// This is not a search
		return
	}

	if p.searchHitIsVisible() {
		// Already on-screen
		return
	}

	if p.scrollRightToSearchHits() {
		// Found it to the right, done!
		return
	}

	lineIndex := p.scrollPosition.lineIndex(p)
	if lineIndex == nil {
		// No lines to search
		return
	}

	firstHitIndex := FindFirstHit(p.Reader(), p.search, *lineIndex, nil, SearchDirectionForward)
	if firstHitIndex == nil {
		alreadyAtTheTop := (*lineIndex == linemetadata.Index{})
		if alreadyAtTheTop {
			// No match, can't wrap, give up
			return
		}

		// Try again from the top
		firstHitIndex = FindFirstHit(p.Reader(), p.search, linemetadata.Index{}, lineIndex, SearchDirectionForward)
	}
	if firstHitIndex == nil {
		// No match, give up
		return
	}

	// Found a match on some line
	p.scrollPosition = NewScrollPositionFromIndex(*firstHitIndex, "scrollToSearchHits")

	p.leftColumnZeroBased = 0
	p.showLineNumbers = p.ShowLineNumbers
	if !p.searchHitIsVisible() {
		p.scrollRightToSearchHits()
	}
	p.centerSearchHitsVertically()
}

// Scroll to the next search hit, when the user presses 'n'.
// wrapAround restarts the search from the top when true, or continues from the
// current position when false.
func (p *Pager) scrollToNextSearchHit(wrapAround bool) {
	if p.search.Inactive() {
		// Nothing to search for, never mind
		return
	}

	if p.Reader().GetLineCount() == 0 {
		// Nothing to search in, never mind
		return
	}

	if p.scrollRightToSearchHits() {
		// Found it to the right, done!
		return
	}

	if !wrapAround && p.isScrolledToEnd() {
		p.mode = PagerModeNotFound{pager: p}
		return
	}

	var firstSearchIndex linemetadata.Index
	if wrapAround {
		// Restart searching from the top; establishing viewing mode here ensures
		// the mode is correct whether we arrive via executeAction (which already
		// switched) or via a direct call (e.g. from tests).
		p.mode = PagerModeViewing{pager: p}
		firstSearchIndex = linemetadata.Index{}
	} else {
		// Start searching on the first line below the bottom of the screen
		firstSearchIndex = *p.getLastVisibleLineIndex()
	}

	firstHitIndex := FindFirstHit(p.Reader(), p.search, firstSearchIndex, nil, SearchDirectionForward)
	if firstHitIndex == nil {
		p.mode = PagerModeNotFound{pager: p}
		return
	}
	p.scrollPosition = NewScrollPositionFromIndex(*firstHitIndex, "scrollToNextSearchHit")

	// Don't let any search hit scroll out of sight
	p.setTargetLine(nil)

	p.leftColumnZeroBased = 0
	p.showLineNumbers = p.ShowLineNumbers
	if !p.searchHitIsVisible() {
		p.scrollRightToSearchHits()
	}
	p.centerSearchHitsVertically()
}

// Scroll backwards to the previous search hit, while the user is typing the
// search string.
func (p *Pager) scrollToSearchHitsBackwards() {
	if p.search.Inactive() {
		// This is not a search
		return
	}

	if p.searchHitIsVisible() {
		// Already on-screen
		return
	}

	if p.scrollLeftToSearchHits() {
		// Found it to the left, done!
		return
	}

	// Start at the top visible line
	lineIndex := p.scrollPosition.lineIndex(p)

	firstHitIndex := FindFirstHit(p.Reader(), p.search, *lineIndex, nil, SearchDirectionBackward)
	if firstHitIndex == nil {
		lastReaderLineIndex := linemetadata.IndexFromLength(p.Reader().GetLineCount())
		if lastReaderLineIndex == nil {
			// In the first part of the search we had some lines to search.
			// Lines should never go away, so this should never happen.
			log.Error("Wrapped backwards search had no lines to search")
			return
		}

		lastVisibleLineIndex := p.getLastVisibleLineIndex()
		canWrap := (*lineIndex != *lastVisibleLineIndex)
		if !canWrap {
			// No match, can't wrap, give up
			return
		}

		// Try again from the bottom
		firstHitIndex = FindFirstHit(p.Reader(), p.search, *lastReaderLineIndex, lineIndex, SearchDirectionBackward)
	}
	if firstHitIndex == nil {
		// No match, give up
		return
	}

	hitPosition := NewScrollPositionFromIndex(*firstHitIndex, "scrollToSearchHitsBackwards")

	// Scroll so that the first hit is at the bottom of the screen. If the
	// visible height is 1, we should scroll 0 steps.
	p.scrollPosition = hitPosition.PreviousLine(p.visibleHeight() - 1)

	p.scrollMaxRight()
	if !p.searchHitIsVisible() {
		p.scrollLeftToSearchHits()
	}
	p.centerSearchHitsVertically()
}

// Scroll backwards to the previous search hit, when the user presses 'N'.
// wrapAround restarts the search from the bottom when true, or continues from
// the current position when false.
func (p *Pager) scrollToPreviousSearchHit(wrapAround bool) {
	if p.search.Inactive() {
		// Nothing to search for, never mind
		return
	}

	if p.Reader().GetLineCount() == 0 {
		// Nothing to search in, never mind
		return
	}

	if p.scrollLeftToSearchHits() {
		// Found it to the left, done!
		return
	}

	var firstSearchIndex linemetadata.Index
	if wrapAround {
		// Restart searching from the bottom; same reasoning as scrollToNextSearchHit.
		p.mode = PagerModeViewing{pager: p}
		firstSearchIndex = *linemetadata.IndexFromLength(p.Reader().GetLineCount())
	} else {
		if p.scrollPosition.lineIndex(p).Index() == 0 {
			// Already at the top, can't go further up
			p.mode = PagerModeNotFound{pager: p}
			return
		}

		// Start searching on the first line above the top of the screen
		position := p.scrollPosition.PreviousLine(1)
		firstSearchIndex = *position.lineIndex(p)
	}

	hitIndex := FindFirstHit(p.Reader(), p.search, firstSearchIndex, nil, SearchDirectionBackward)
	if hitIndex == nil {
		p.mode = PagerModeNotFound{pager: p}
		return
	}
	p.scrollPosition = *scrollPositionFromIndex("scrollToPreviousSearchHit", *hitIndex)

	// Don't let any search hit scroll out of sight
	p.setTargetLine(nil)

	// Prefer hits to the right
	p.scrollMaxRight()
	if !p.searchHitIsVisible() {
		p.scrollLeftToSearchHits()
	}
	p.centerSearchHitsVertically()
}

// Return true if any search hit is currently visible on screen.
//
// A search hit is considered visible if the first character of the hit is
// visible. This means that if the hit is longer than one character, the rest of
// it may be off-screen to the right. If that happens, the user can scroll right
// manually to see the rest of the hit.
func (p *Pager) searchHitIsVisible() bool {
	for _, row := range p.renderLines().lines {
		for _, cell := range row.cells {
			if cell.StartsSearchHit {
				// Found a search hit on screen!
				return true
			}
		}
	}

	// No search hits found
	return false
}

func (p *Pager) centerSearchHitsVertically() {
	if p.WrapLongLines {
		// FIXME: Centering is not supported when wrapping, future improvement!
		return
	}

	for {
		rendered := p.renderLines()
		firstHitRow := linemetadata.ScreenLines(-1)
		lastHitRow := linemetadata.ScreenLines(-1)
		for rowIndex, row := range rendered.inputLines {
			if !p.search.Matches(row.Plain()) {
				continue
			}

			if firstHitRow == -1 {
				firstHitRow = linemetadata.ScreenLines(rowIndex)
			}
			lastHitRow = linemetadata.ScreenLines(rowIndex)
		}

		if firstHitRow == -1 || lastHitRow == -1 {
			log.Warn("No hits found while centering, how did we get here?")
			return
		}

		// If the visible height is 1, the center screen row is 0.
		centerScreenRowDoubled := p.visibleHeight() - 1

		centerHitRowDoubled := firstHitRow + lastHitRow

		// Divide by 2 here to get the amount of rows we need to scroll. We
		// postponed the division by 2 until now to avoid rounding errors.
		//
		// If the center screen row is 1 (3 lines visible), and the center hit
		// row is 2 (last screen line), we need to arrow down once.
		deltaRows := (centerHitRowDoubled - centerScreenRowDoubled) / 2

		newScrollPosition := p.scrollPosition.NextLine(deltaRows)
		if p.ScrollPositionsEqual(p.scrollPosition, newScrollPosition) {
			// No change, done!
			return
		}

		p.scrollPosition = newScrollPosition
	}
}

// If we are alredy too far right when you call this method, it will scroll
// left.
func (p *Pager) scrollMaxRight() {
	if p.WrapLongLines {
		// No horizontal scrolling when wrapping
		return
	}

	// First, render a screen scrolled to the far left so we know how much space
	// line numbers take.
	p.leftColumnZeroBased = 0
	p.showLineNumbers = p.ShowLineNumbers
	rendered := p.renderLines()

	// Find the widest line, in screen cells. Some runes are double-width.
	widestLineWidth := 0
	for _, inputLine := range rendered.inputLines {
		lineLength := inputLine.DisplayWidth()
		if lineLength > widestLineWidth {
			widestLineWidth = lineLength
		}
	}

	screenWidth, _ := p.screen.Size()

	availableWidth := screenWidth - rendered.numberPrefixWidth
	if widestLineWidth <= availableWidth {
		// All lines fit on screen, this means we're now max scrolled right
		return
	}

	p.showLineNumbers = false
	availableWidth += rendered.numberPrefixWidth
	if widestLineWidth <= availableWidth {
		// All lines fit on screen with line numbers off, this means we're now
		// max scrolled right
		return
	}

	// If the line width is 10 and the available width is also 10, we should
	// start at column 0.
	p.leftColumnZeroBased = widestLineWidth - availableWidth
}

// Scroll right looking for search hits. Return true if we found any.
func (p *Pager) scrollRightToSearchHits() bool {
	if p.WrapLongLines {
		// No horizontal scrolling when wrapping
		return false
	}

	restoreShowLineNumbers := p.showLineNumbers
	restoreLeftColumn := p.leftColumnZeroBased

	// Check how far right we can scroll at most. Factors involved:
	// - Screen width
	// - Length of longest visible line
	screenWidth, _ := p.screen.Size()

	rendered := p.renderLines()

	if screenWidth-2-rendered.numberPrefixWidth < 0 {
		log.Infof("Screen too narrow (%d) to scroll right for search hits, skipping", screenWidth)
		return false
	}

	widestLineWidth := 0 // In screen cells, some runes are double-width
	for _, inputLine := range rendered.inputLines {
		lineLength := inputLine.DisplayWidth()
		if lineLength > widestLineWidth {
			widestLineWidth = lineLength
		}
	}

	// With a 10 wide screen and a 15 wide line (max index 14), the leftmost
	// screen column can at most be 5:
	//
	// Screen column: 0123456789
	// Line column:   5678901234
	maxLeftmostColumn := widestLineWidth - screenWidth
	if maxLeftmostColumn < 0 {
		maxLeftmostColumn = 0
	}

	firstNotVisibleColumn := p.leftColumnZeroBased + screenWidth - rendered.numberPrefixWidth - 1
	if firstNotVisibleColumn < 1 {
		firstNotVisibleColumn = 1
	}

	// Find the leftmost search hit column that is not visible yet. If there are
	// no more hits to the right, minHitCol will stay -1.
	minHitCol := -1
	for _, inputLine := range rendered.inputLines {
		matches := p.search.GetMatchRanges(inputLine.Plain())
		if matches != nil {
			runes := []rune(inputLine.Plain())
			for _, hit := range matches.Matches {
				hitCol := uniseg.StringWidth(string(runes[:hit[0]]))
				if hitCol >= firstNotVisibleColumn {
					if minHitCol == -1 || hitCol < minHitCol {
						minHitCol = hitCol
					}
				}
			}
		}
	}

	if minHitCol == -1 {
		// Can't scroll right, pretend nothing happened
		p.showLineNumbers = restoreShowLineNumbers
		p.leftColumnZeroBased = restoreLeftColumn
		return false
	}

	p.showLineNumbers = false

	// Scroll horizontally so that the search hit is centered on the
	// screen. This provides some context both before and after the hit.
	scrollToColumn := minHitCol - screenWidth/2
	if scrollToColumn < 0 {
		scrollToColumn = 0
	}

	if p.leftColumnZeroBased == scrollToColumn && p.showLineNumbers == restoreShowLineNumbers {
		// Nothing changed, done!
		return false
	}

	p.leftColumnZeroBased = scrollToColumn

	// A new hit showed up!
	if p.leftColumnZeroBased > maxLeftmostColumn {
		// Scrolled beyond max, adjust
		p.leftColumnZeroBased = maxLeftmostColumn
	}
	return true
}

// Scroll left looking for search hits. Return true if we found any.
func (p *Pager) scrollLeftToSearchHits() bool {
	if p.WrapLongLines {
		// No horizontal scrolling when wrapping
		return false
	}

	restoreLeftColumn := p.leftColumnZeroBased
	restoreShowLineNumbers := p.showLineNumbers

	screenWidth, _ := p.screen.Size()

	// Find the rightmost search hit column that is strictly to the left of the
	// currently visible area.
	maxHitCol := -1

	// We only care about hits that are to the left of leftColumnZeroBased.
	// That is, hits that start at hitCol < p.leftColumnZeroBased.

	rendered := p.renderLines()

	for _, inputLine := range rendered.inputLines {
		matches := p.search.GetMatchRanges(inputLine.Plain())
		if matches != nil {
			runes := []rune(inputLine.Plain())
			for _, hit := range matches.Matches {
				hitCol := uniseg.StringWidth(string(runes[:hit[0]]))
				if hitCol < p.leftColumnZeroBased {
					if hitCol > maxHitCol {
						maxHitCol = hitCol
					}
				}
			}
		}
	}

	// If we go max left, which column will be the rightmost visible one?
	var fullLeftRightmostVisibleColumn int
	{
		p.showLineNumbers = p.ShowLineNumbers
		p.leftColumnZeroBased = 0
		renderedLeft := p.renderLines()
		// If the screen width is 2, we have columns 0 and 1. The rightmost column can be covered by
		// scroll-right markers, so the first not-visible column when fully scrolled left is 0, or
		// "2 - 2".
		fullLeftRightmostVisibleColumn = screenWidth - 2 - renderedLeft.numberPrefixWidth

		p.leftColumnZeroBased = restoreLeftColumn
		p.showLineNumbers = restoreShowLineNumbers
	}

	if fullLeftRightmostVisibleColumn < 0 {
		log.Info("Screen too narrow ({}) to scroll left for search hits, skipping", screenWidth)
		return false
	}

	if maxHitCol == -1 {
		// Can't scroll left to a hit, pretend nothing happened
		p.showLineNumbers = restoreShowLineNumbers
		p.leftColumnZeroBased = restoreLeftColumn
		return false
	}

	// We found a hit at maxHitCol to the left.

	// If the hit is at or before what would be visible when fully scrolled
	// left, just go fully left.
	if maxHitCol <= fullLeftRightmostVisibleColumn {
		p.showLineNumbers = p.ShowLineNumbers
		p.leftColumnZeroBased = 0
		return true
	}

	// Otherwise, scroll so that maxHitCol becomes visible. Since we aren't
	// scrolled max left, we line numbers are off.
	p.showLineNumbers = false

	// Scroll horizontally so that the search hit is centered on the
	// screen. This provides some context both before and after the hit.
	scrollToColumn := maxHitCol - screenWidth/2
	if scrollToColumn < 0 {
		scrollToColumn = 0
	}

	p.leftColumnZeroBased = scrollToColumn

	return true
}

func (p *Pager) isViewing() bool {
	_, isViewing := p.mode.(PagerModeViewing)
	return isViewing
}
