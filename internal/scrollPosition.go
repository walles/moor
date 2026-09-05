package internal

import (
	"fmt"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
)

// Says which line should be at the top of the screen.
//
// Using two separate numbers:
//   - A line index into the input stream (lineIndex)
//   - A delta of how many screen lines to scroll down before rendering. This is
//     needed to support scrolling through wrapped lines.
//
// If you put the scroll position too far down, it will be adjusted so no empty
// lines are shown at the bottom of the screen.
//
// Please create using newScrollPosition(name).
type scrollPosition struct {
	internalDontTouch scrollPositionInternal
}

// getMaxNumberPrefixLength returns the maximum line number prefix length
// required for the lines currently visible on screen. It does not return the
// prefix length for the whole input stream, but rather an approximation based
// on the highest input line index currently displayed.
func (sp scrollPosition) getMaxNumberPrefixLength(pager *Pager) int {
	return sp.internalDontTouch.getMaxNumberPrefixLength(pager)
}

func newScrollPosition(name string) scrollPosition {
	if len(name) == 0 {
		panic("Non-empty name required")
	}
	return scrollPosition{
		internalDontTouch: scrollPositionInternal{
			name: name,
		},
	}
}

type scrollPositionInternal struct {
	// Index into the array of visible input lines, or nil if nothing has been
	// read yet or there are no lines. This is the logical line at the top of
	// the screen.
	lineIndex *linemetadata.Index

	// Scroll this many screen lines before rendering. Can be negative during
	// math.
	//
	// A single logical lineIndex can wrap into multiple ScreenLines. delta
	// tracks how many ScreenLines of the top lineIndex are hidden above the top
	// edge of the terminal.
	delta linemetadata.ScreenLines

	name           string
	canonicalizing bool
	canonical      scrollPositionCanonical
}

// If any of these change, we have to recompute the scrollPositionInternal values
type scrollPositionCanonical struct {
	width           int                      // From pager
	height          linemetadata.ScreenLines // From pager
	showLineNumbers bool                     // From pager
	showStatusBar   bool                     // From pager
	wrapLongLines   bool                     // From pager

	pagerLineCount int // From pager.Reader().GetLineCount()

	lineIndex        *linemetadata.Index      // From scrollPositionInternal
	deltaScreenLines linemetadata.ScreenLines // From scrollPositionInternal
}

func canonicalFromPager(pager *Pager) scrollPositionCanonical {
	width, _ := pager.screen.Size()
	height := pager.visibleHeight()
	return scrollPositionCanonical{
		width:           width,
		height:          height,
		showLineNumbers: pager.showLineNumbers,
		showStatusBar:   pager.ShowStatusBar,
		wrapLongLines:   pager.WrapLongLines,

		pagerLineCount: pager.Reader().GetLineCount(),

		lineIndex:        pager.scrollPosition.internalDontTouch.lineIndex,
		deltaScreenLines: pager.scrollPosition.internalDontTouch.delta,
	}
}

// Create a new position, scrolled towards the beginning of the file
func (sp scrollPosition) PreviousLine(scrollDistance linemetadata.ScreenLines) scrollPosition {
	return scrollPosition{
		internalDontTouch: scrollPositionInternal{
			name:      sp.internalDontTouch.name,
			lineIndex: sp.internalDontTouch.lineIndex,
			delta:     sp.internalDontTouch.delta - scrollDistance,
		},
	}
}

// Create a new position, scrolled towards the end of the file
func (sp scrollPosition) NextLine(scrollDistance linemetadata.ScreenLines) scrollPosition {
	return scrollPosition{
		internalDontTouch: scrollPositionInternal{
			name:      sp.internalDontTouch.name,
			lineIndex: sp.internalDontTouch.lineIndex,
			delta:     sp.internalDontTouch.delta + scrollDistance,
		},
	}
}

func (p *Pager) ScrollPositionsEqual(a, b scrollPosition) bool {
	a.internalDontTouch.canonicalize(p)
	b.internalDontTouch.canonicalize(p)
	if a.internalDontTouch.delta != b.internalDontTouch.delta {
		return false
	}

	if (a.internalDontTouch.lineIndex == nil) != (b.internalDontTouch.lineIndex == nil) {
		// One line index is nil, the other is not
		return false
	}

	if a.internalDontTouch.lineIndex != nil && b.internalDontTouch.lineIndex != nil {
		// Both line indexes are non-nil...
		if *a.internalDontTouch.lineIndex != *b.internalDontTouch.lineIndex {
			// ... but they point to different lines.
			return false
		}
	}

	return true
}

// Create a new position, scrolled to the given line number
//
//revive:disable-next-line:unexported-return
func NewScrollPositionFromIndex(index linemetadata.Index, name string) scrollPosition {
	return scrollPosition{
		internalDontTouch: scrollPositionInternal{
			name:      name,
			lineIndex: &index,
			delta:     0,
		},
	}
}

// Move towards the top until deltaScreenLines is not negative any more
func (si *scrollPositionInternal) handleNegativeDelta(pager *Pager) {
	if si.lineIndex == nil {
		si.delta = 0
		return
	}
	for !si.lineIndex.IsZero() && si.delta < 0 {
		// Render the previous line
		previousLineIndex := si.lineIndex.NonWrappingAdd(-1)
		previousLine := pager.Reader().GetLine(previousLineIndex)
		previousSubLinesCount := linemetadata.ScreenLines(0)
		if previousLine != nil {
			previousSubLines := pager.renderLine(*previousLine, si.getMaxNumberPrefixLength(pager), true)
			previousSubLinesCount = linemetadata.ScreenLines(len(previousSubLines))
		}

		// Adjust lineNumber and deltaScreenLines to move up into the previous
		// screen line
		si.lineIndex = &previousLineIndex
		si.delta += previousSubLinesCount
	}

	if si.lineIndex.IsZero() && si.delta <= 0 {
		// Can't go any higher
		si.delta = 0
		return
	}
}

// Move towards the bottom until deltaScreenLines is within range of the
// rendering of the current line.
//
// This method will not do any screen-height based clipping, so it could be that
// the position is too far down to display after this returns.
func (si *scrollPositionInternal) handlePositiveDelta(pager *Pager) {
	maxPrefixLength := si.getMaxNumberPrefixLength(pager)

	for {
		line := pager.Reader().GetLine(*si.lineIndex)
		if line == nil {
			// Out of bounds downwards, get the last line...
			si.lineIndex = linemetadata.IndexFromLength(pager.Reader().GetLineCount())
			if si.lineIndex == nil {
				// Can happen if the line count shrunk to 0 while processing.
				// Filtering readers can do this, for example.
				si.delta = 0
				return
			}

			line = pager.Reader().GetLine(*si.lineIndex)
			if line == nil {
				// Can happen if the line count shrunk to 0 while processing.
				// Filtering readers can do this, for example.
				si.delta = 0
				return
			}

			subLines := pager.renderLine(*line, maxPrefixLength, true)

			// ... and go to the bottom of that.
			si.delta = linemetadata.ScreenLines(len(subLines) - 1)
			return
		}

		subLines := pager.renderLine(*line, maxPrefixLength, true)
		if si.delta < linemetadata.ScreenLines(len(subLines)) {
			// Sublines are within bounds!
			return
		}

		si.lineIndex = ptr.To(si.lineIndex.NonWrappingAdd(1))
		si.delta -= linemetadata.ScreenLines(len(subLines))
	}
}

// This method assumes si contains a canonical position
func (si *scrollPositionInternal) emptyBottomLinesCount(pager *Pager) linemetadata.ScreenLines {
	unclaimedViewportLines := pager.visibleHeight()
	if unclaimedViewportLines == 0 {
		// No lines at all => no lines are empty. Happens (at least) during
		// testing.
		return 0
	}
	if pager.Reader().GetLineCount() == 0 {
		// No lines available, so all viewport lines are unclaimed
		return unclaimedViewportLines
	}

	// Start counting where the current input line begins
	unclaimedViewportLines += si.delta

	lineIndex := *si.lineIndex

	lastLineNumberWidth := si.getMaxNumberPrefixLength(pager)

	for {
		line := pager.Reader().GetLine(lineIndex)
		if line == nil {
			// No more lines!
			break
		}

		subLines := pager.renderLine(*line, lastLineNumberWidth, true)
		unclaimedViewportLines -= linemetadata.ScreenLines(len(subLines))
		if unclaimedViewportLines <= 0 {
			return 0
		}

		// Move to the next line
		lineIndex = lineIndex.NonWrappingAdd(1)
	}

	return unclaimedViewportLines
}

func (si *scrollPositionInternal) isCanonical(pager *Pager) bool {
	if si.canonical.lineIndex == nil {
		// Awaiting initial lines from the reader
		return false
	}

	if si.canonical == canonicalFromPager(pager) {
		return true
	}

	return false
}

// Only to be called from the scrollPosition getters!!
//
// Canonicalize the scroll position vs the given pager. A canonical position can
// just be displayed on screen, it has been clipped both towards the top and
// bottom of the screen, taking into account the screen height.
//
// It guarantees the following invariants:
//  1. If the pager is empty, lineIndex is nil and deltaScreenLines is 0.
//  2. deltaScreenLines is always >= 0 and strictly less than the number of
//     visual wrapped lines produced by rendering lineIndex.
//  3. The screen will not have an empty gap at the bottom if there is
//     enough text in the input stream to fill it.
func (si *scrollPositionInternal) canonicalize(pager *Pager) {
	if si.isCanonical(pager) {
		return
	}

	if si.canonicalizing {
		panic(fmt.Errorf("Scroll position canonicalize() called recursively for %s", si.name))
	}
	si.canonicalizing = true

	defer func() {
		si.canonical = canonicalFromPager(pager)
		si.canonicalizing = false
	}()

	if pager.Reader().GetLineCount() == 0 {
		si.lineIndex = nil
		si.delta = 0
		return
	}

	if si.lineIndex == nil {
		// We have lines, but no line number, start at the top
		si.lineIndex = &linemetadata.Index{}
	}

	si.handleNegativeDelta(pager)
	si.handlePositiveDelta(pager)
	emptyBottomLinesCount := si.emptyBottomLinesCount(pager)
	if emptyBottomLinesCount > 0 {
		// First, adjust deltaScreenLines to get us to the top
		si.delta -= emptyBottomLinesCount

		// Then, actually go up that many lines
		si.handleNegativeDelta(pager)
	}
}

func scrollPositionFromIndex(name string, index linemetadata.Index) *scrollPosition {
	return &scrollPosition{
		internalDontTouch: scrollPositionInternal{
			name:      name,
			lineIndex: &index,
		},
	}
}

// Line index in the input stream, or nil if nothing has been read
func (p *Pager) lineIndex() *linemetadata.Index {
	p.scrollPosition.internalDontTouch.canonicalize(p)
	return p.scrollPosition.internalDontTouch.lineIndex
}

// Line index in the input stream, or nil if nothing has been read
func (sp *scrollPosition) lineIndex(pager *Pager) *linemetadata.Index {
	sp.internalDontTouch.canonicalize(pager)
	return sp.internalDontTouch.lineIndex
}

// Scroll this many screen lines before rendering
//
// Always >= 0.
func (p *Pager) deltaScreenLines() linemetadata.ScreenLines {
	p.scrollPosition.internalDontTouch.canonicalize(p)
	return p.scrollPosition.internalDontTouch.delta
}

func (p *Pager) scrollToEnd() {
	inputLineCount := p.Reader().GetLineCount()
	if inputLineCount == 0 {
		return
	}
	lastInputIndex := *linemetadata.IndexFromLength(inputLineCount)

	lastInputLine := p.Reader().GetLine(lastInputIndex)

	p.scrollPosition.internalDontTouch.lineIndex = &lastInputIndex

	// Scroll down enough. We know for sure the last line won't wrap into more
	// lines than the number of characters it contains.
	p.scrollPosition.internalDontTouch.delta = linemetadata.ScreenLines(len(lastInputLine.Line.Plain(lastInputIndex)))

	if p.TargetLine == nil {
		// Start following the end of the file
		//
		// Otherwise, if we're already aiming for some place, don't overwrite
		// that.
		p.setTargetLine(ptr.To(linemetadata.IndexMax()))
	}
}

// Can be either because Pager.scrollToEnd() was just called or because the user
// has pressed the down arrow enough times.
func (p *Pager) isScrolledToEnd() bool {
	inputLineCount := p.Reader().GetLineCount()
	if inputLineCount == 0 {
		// No lines available, which means we can't scroll any further down
		return true
	}
	lastInputLineIndex := *linemetadata.IndexFromLength(inputLineCount)

	visibleLines := p.renderLines().lines
	if len(visibleLines) == 0 {
		// This can happen when terminal height is 1. In that case, the status
		// bar takes up that single line, leaving no room for contents.
		//
		// The actual return value here was picked based on that it prevents the
		// crash reported here: https://github.com/walles/moor/issues/378
		return true
	}

	lastVisibleLine := visibleLines[len(visibleLines)-1]
	if lastVisibleLine.inputLineIndex != lastInputLineIndex {
		// Last input line is not on the screen
		return false
	}

	// Last line is on screen, now we need to figure out whether we can see all
	// of it
	lastInputLine := p.Reader().GetLine(lastInputLineIndex)
	lastInputLineRendered := p.renderLine(*lastInputLine, p.getLineNumberPrefixLength(lastInputLine.Number), true)
	lastRenderedSubLine := lastInputLineRendered[len(lastInputLineRendered)-1]

	// If the last visible subline is the same as the last possible subline then
	// we're at the bottom
	return lastVisibleLine.wrapIndex == lastRenderedSubLine.wrapIndex
}

// Returns nil if there are no lines
func (p *Pager) getLastVisibleLineIndex() *linemetadata.Index {
	rendered := p.renderLines()
	if len(rendered.lines) == 0 {
		return nil
	}

	return ptr.To(rendered.lines[len(rendered.lines)-1].inputLineIndex)
}

// getMaxNumberPrefixLength returns the maximum line number prefix length
// required for the lines currently visible on screen. It does not return the
// prefix length for the whole input stream, but rather an approximation based
// on the highest input line index currently displayed.
//
// There is a circular dependency when calculating gutter widths:
//  1. To know how many screen lines a logical line wraps into, you must know
//     available horizontal space.
//  2. To know horizontal space, you must know the width of the line number gutter.
//  3. To know gutter width, you must know the highest line number visible on screen.
//  4. To know which lines are visible, you must know how many wraps are occurring!
//
// This function breaks the cycle by providing a fast, worst-case bounded
// approximation, assuming 1 logical line = 1 screen line.
func (si *scrollPositionInternal) getMaxNumberPrefixLength(pager *Pager) int {
	maxPossibleIndexPtr := linemetadata.IndexFromLength(pager.Reader().GetLineCount())
	if maxPossibleIndexPtr == nil {
		return 0
	}
	maxPossibleIndex := *maxPossibleIndexPtr

	// This is an approximation assuming we don't do any wrapping. Finding the
	// real answer while wrapping requires rendering, which requires the real
	// answer and so on, so we do an approximation here to save us from
	// recursion.
	//
	// Let's improve on demand.
	var index linemetadata.Index
	// Ref: https://github.com/walles/moor/issues/198
	if si.lineIndex != nil {
		index = *si.lineIndex
	}

	// Not going back when computing the max number prefix length prevents
	// https://github.com/walles/moor/issues/338.
	//
	// FIXME: Is there something better we should be doing instead?
	positiveDelta := si.delta
	if positiveDelta < 0 {
		positiveDelta = 0
	}

	maxVisibleIndex := index.NonWrappingAdd(
		int(positiveDelta + pager.visibleHeight() - 1))
	if maxVisibleIndex.IsAfter(maxPossibleIndex) {
		maxVisibleIndex = maxPossibleIndex
	}

	var number linemetadata.Number
	lastVisibleLine := pager.Reader().GetLine(maxVisibleIndex)

	// nil can happen when the input stream is empty
	if lastVisibleLine != nil {
		number = lastVisibleLine.Number
	}

	// Count the length of the last line number
	return pager.getLineNumberPrefixLength(number)
}
