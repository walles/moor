package internal

import (
	"fmt"
	"slices"

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
func (p *Pager) scrollToNextSearchHit() {
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

	if p.isViewing() && p.isScrolledToEnd() {
		p.mode = PagerModeNotFound{pager: p}
		return
	}

	var firstSearchIndex linemetadata.Index

	switch {
	case p.isViewing():
		// Start searching on the first line below the bottom of the screen
		firstSearchIndex = *p.getLastVisibleLineIndex()

	case p.isNotFound():
		// Restart searching from the top
		p.mode = PagerModeViewing{pager: p}
		firstSearchIndex = linemetadata.Index{}

	default:
		panic(fmt.Sprint("Unknown search mode when finding next: ", p.mode))
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
func (p *Pager) scrollToPreviousSearchHit() {
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

	switch {
	case p.isViewing():
		if p.scrollPosition.lineIndex(p).Index() == 0 {
			// Already at the top, can't go further up
			p.mode = PagerModeNotFound{pager: p}
			return
		}

		// Start searching on the first line above the top of the screen
		position := p.scrollPosition.PreviousLine(1)
		firstSearchIndex = *position.lineIndex(p)

	case p.isNotFound():
		// Restart searching from the bottom
		p.mode = PagerModeViewing{pager: p}
		firstSearchIndex = *linemetadata.IndexFromLength(p.Reader().GetLineCount())

	default:
		panic(fmt.Sprint("Unknown search mode when finding previous: ", p.mode))
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
		if row.containsSearchHit {
			return true
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

	rendered := p.renderLines()
	width, _ := p.screen.Size()
	offscreenColumn := p.leftColumnZeroBased + width + 1
	var nextHitOffset []int
	for _, line := range rendered.lines {
		for _, hit := range line.searchHits.Matches {
			if hit[0] > offscreenColumn {
				nextHitOffset = append(nextHitOffset, hit[0])
			}
		}
	}

	// No hits at all in these lines.
	if len(nextHitOffset) == 0 {
		return false
	}

	// Move the left size to the next most hit.
	p.leftColumnZeroBased = slices.Min(nextHitOffset)

	return true
}

// Scroll left looking for search hits. Return true if we found any.
func (p *Pager) scrollLeftToSearchHits() bool {
	if p.WrapLongLines {
		// No horizontal scrolling when wrapping
		return false
	}

	rendered := p.renderLines()
	offscreenColumn := p.leftColumnZeroBased
	var nextHitOffset []int
	for _, line := range rendered.lines {
		for _, hit := range line.searchHits.Matches {
			if hit[0] < offscreenColumn {
				nextHitOffset = append(nextHitOffset, hit[0])
			}
		}
	}

	// No hits at all in these lines.
	if len(nextHitOffset) == 0 {
		return false
	}

	// Move the left size to the next most hit.
	p.leftColumnZeroBased = slices.Max(nextHitOffset)

	return true
}

func (p *Pager) isViewing() bool {
	_, isViewing := p.mode.(PagerModeViewing)
	return isViewing
}

func (p *Pager) isNotFound() bool {
	_, isNotFound := p.mode.(PagerModeNotFound)
	return isNotFound
}
