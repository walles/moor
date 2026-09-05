package internal

import (
	"fmt"

	"github.com/davecgh/go-spew/spew"
	log "github.com/sirupsen/logrus"
	"github.com/walles/twin"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/textstyles"
)

type renderedLine struct {
	// Certain lines are available for viewing. This index is the (zero based)
	// position of this line among those.
	inputLineIndex linemetadata.Index

	// If an input line has been wrapped into two, the part on the second line
	// will have a wrapIndex of 1.
	wrapIndex linemetadata.ScreenLines

	containsSearchHit bool

	cells textstyles.CellWithMetadataSlice

	// Used for rendering clear-to-end-of-line control sequences:
	// https://en.wikipedia.org/wiki/ANSI_escape_code#EL
	//
	// Ref: https://github.com/walles/moor/issues/106
	trailer twin.Style
}

type renderedScreen struct {
	lines             []renderedLine
	inputLines        []reader.NumberedLine
	numberPrefixWidth int // Including padding. 0 means no line numbers.
	reader.Status
}

// Refresh the whole pager display, both contents lines and the status line at
// the bottom, and show the result on the terminal
func (p *Pager) redraw(spinner string) {
	p.renderIntoCells(spinner)
	p.screen.Show()
}

// Render the whole pager display into the screen's cells, both contents lines
// and the status line at the bottom.
//
// Nothing reaches the terminal until somebody Show()s the screen.
//
// Returns the number of screen lines the contents ended up on, not counting the
// status line below them.
func (p *Pager) renderIntoCells(spinner string) int {
	log.Trace("renderIntoCells called")
	p.screen.Clear()
	p.longestLineLength = 0

	lastUpdatedScreenLineNumber := -1
	renderedScreen := p.renderLines()
	for screenLineNumber, row := range renderedScreen.lines {
		lastUpdatedScreenLineNumber = screenLineNumber
		column := 0
		for _, cell := range row.cells {
			column += p.screen.SetCell(column, lastUpdatedScreenLineNumber, cell.ToStyledRune())
		}
	}

	// Status line code follows

	eofSpinner := spinner
	if eofSpinner == "" {
		// This happens when we're done
		eofSpinner = "---"
	}
	spinnerLine := textstyles.StyledRunesFromString(theme.statusbar, eofSpinner, nil, 0).StyledRunes
	column := 0
	for _, cell := range spinnerLine {
		column += p.screen.SetCell(column, lastUpdatedScreenLineNumber+1, cell.ToStyledRune())
	}

	p.mode.drawFooter(renderedScreen.FilenameText, renderedScreen.StatusText, spinner)

	if spinner != "" {
		p.screen.SetProgress(twin.ProgressStateIndeterminate, 0)
	} else {
		p.screen.SetProgress(twin.ProgressStateRemove, 0)
	}

	return len(renderedScreen.lines)
}

// Render all lines that should go on the screen.
//
// Returns both the lines and a suitable status text.
//
// The returned lines are display ready, meaning that they come with horizontal
// scroll markers and line numbers as necessary.
//
// The maximum number of lines returned by this method is limited by the screen
// height. If the status line is visible, you'll get at most one less than the
// screen height from this method.
func (p *Pager) renderLines() renderedScreen {
	rendered := p.internalRenderLines(true)
	hasSearchHitLines := false
	hasNonSearchHitLines := false
	for _, line := range rendered.lines {
		if line.containsSearchHit {
			hasSearchHitLines = true
		} else {
			hasNonSearchHitLines = true
		}
	}

	if hasSearchHitLines && !hasNonSearchHitLines {
		// All lines have search hits, don't highlight any lines
		// Ref: https://github.com/walles/moor/issues/335
		rendered = p.internalRenderLines(false)
	}

	return rendered
}

func (p *Pager) internalRenderLines(highlightSearchHitLines bool) renderedScreen {
	var lineIndexToShow linemetadata.Index
	if p.lineIndex() != nil {
		lineIndexToShow = *p.lineIndex()
	}
	inputLines := p.Reader().GetLines(lineIndexToShow, int(p.visibleHeight()))
	if len(inputLines.Lines) == 0 {
		// Empty input, empty output
		return renderedScreen{Status: inputLines.Status}
	}

	// We must use the exact same max number prefix length as the scroll engine
	// used to calculate its viewport. If the engine used an approximation, we
	// must use the exact same approximation, otherwise the number of line wraps
	// might disagree, causing panics (Issue #399).
	numberPrefixLength := p.scrollPosition.getMaxNumberPrefixLength(p)

	allLines := make([]renderedLine, 0)
	for _, line := range inputLines.Lines {
		rendering := p.renderLine(line, numberPrefixLength, highlightSearchHitLines)

		var onScreenLength int
		for i := range rendering {
			trimmedLen := len(rendering[i].cells.WithoutSpaceRight())
			if trimmedLen > onScreenLength {
				onScreenLength = trimmedLen
			}
		}

		// We're trying to find the max length of readable characters to limit
		// the scrolling to right, so we don't go over into the vast emptiness for no reason.
		//
		// The -1 fixed an issue that seemed like an off-by-one where sometimes, when first
		// scrolling completely to the right, the first left scroll did not show the text again.
		displayLength := p.leftColumnZeroBased + onScreenLength - 1

		if displayLength >= p.longestLineLength {
			p.longestLineLength = displayLength
		}

		allLines = append(allLines, rendering...)
	}

	// Find which index in allLines the user wants to see at the top of the
	// screen
	firstVisibleIndex := -1 // Not found
	for index, line := range allLines {
		if p.lineIndex() == nil {
			// Expected zero lines but got some anyway, grab the first one!
			firstVisibleIndex = index
			break
		}
		if line.inputLineIndex == *p.lineIndex() && line.wrapIndex == p.deltaScreenLines() {
			firstVisibleIndex = index
			break
		}
	}
	if firstVisibleIndex == -1 {
		allLinesDescription := fmt.Sprintf("size %d", len(allLines))
		if len(allLines) > 0 {
			allLinesDescription += fmt.Sprintf(", indexed %d-%d:\n",
				allLines[0].inputLineIndex, allLines[len(allLines)-1].inputLineIndex)
		}
		panic(fmt.Errorf("scrollPosition index=%d not found in allLines %s:\n%s",
			lineIndexToShow,
			allLinesDescription,
			spew.Sdump(p.scrollPosition),
		))
	}

	// Drop the lines that should go above the screen
	allLines = allLines[firstVisibleIndex:]

	// Drop the lines that would have gone below the screen
	wantedLineCount := int(p.visibleHeight())
	if len(allLines) > wantedLineCount {
		allLines = allLines[0:wantedLineCount]
	}

	// Fill in the line trailers
	screenWidth, _ := p.screen.Size()
	for i := range allLines {
		line := &allLines[i]
		if line.trailer == twin.StyleDefault {
			continue
		}

		for len(line.cells) < screenWidth {
			line.cells = append(line.cells, textstyles.CellWithMetadata{Rune: ' ', Style: line.trailer})
		}
	}

	return renderedScreen{
		lines:             allLines,
		Status:            inputLines.Status,
		inputLines:        inputLines.Lines,
		numberPrefixWidth: numberPrefixLength,
	}
}

// Render one input line into one or more screen lines.
//
// The returned line is display ready, meaning that it comes with horizontal
// scroll markers and line number as necessary.
//
// lineNumber and numberPrefixLength are required for knowing how much to
// indent, and to (optionally) render the line number.
func (p *Pager) renderLine(line reader.NumberedLine, numberPrefixLength int, highlightSearchHitLines bool) []renderedLine {
	width, _ := p.screen.Size()
	if p.WrapLongLines && p.Width > 0 && p.Width < width {
		width = p.Width
	}
	var wrapped []textstyles.StyledRunesWithTrailer
	var highlighted textstyles.StyledRunesWithTrailer
	if p.WrapLongLines {
		highlighted = line.HighlightedTokens(theme.plainText, theme.searchHit, p.search, 0)

		wrapped = wrapLine(width-numberPrefixLength, highlighted.StyledRunes)
	} else {
		// Request only screen width tokens plus whatever is needed on the left
		// due to horizontal scrolling. Also, get one extra to the right so we
		// can know whether to show overflow markers.
		//
		// This is a huge performance gain when dealing with files with
		// extremeny long lines: https://github.com/walles/moor/issues/358
		highlighted = line.HighlightedTokens(theme.plainText, theme.searchHit, p.search, width+p.leftColumnZeroBased+1)

		// All on one line
		wrapped = []textstyles.StyledRunesWithTrailer{{
			StyledRunes:       highlighted.StyledRunes,
			Trailer:           highlighted.Trailer,
			ContainsSearchHit: highlighted.ContainsSearchHit,
		}}
	}

	if highlightSearchHitLines && theme.searchHitLineBackground != nil {
		// Highlight any sub lines with search hits
		for i := range wrapped {
			line := &wrapped[i] // We need a pointer to modify in place, otherwise setting the trailer won't have any effect
			if line.ContainsSearchHit {
				// Highlight this line!
				for i := range line.StyledRunes {
					line.StyledRunes[i].Style = line.StyledRunes[i].Style.WithBackground(*theme.searchHitLineBackground)
				}
				line.Trailer = line.Trailer.WithBackground(*theme.searchHitLineBackground)
			}
		}
	}

	rendered := make([]renderedLine, 0)
	for wrapIndex, subLine := range wrapped {
		visibleLineNumber := ptr.To(line.Number)
		if wrapIndex > 0 {
			visibleLineNumber = nil
		}

		decorated := p.decorateLine(visibleLineNumber, numberPrefixLength, subLine.StyledRunes)

		rendered = append(rendered, renderedLine{
			inputLineIndex:    line.Index,
			wrapIndex:         linemetadata.ScreenLines(wrapIndex),
			cells:             decorated,
			containsSearchHit: subLine.ContainsSearchHit,
			trailer:           subLine.Trailer,
		})
	}

	lastRenderedLine := &rendered[len(rendered)-1]
	lastLineHasSearchHighlight := highlightSearchHitLines && lastRenderedLine.containsSearchHit
	if highlighted.Trailer != twin.StyleDefault && !lastLineHasSearchHighlight {
		// In the presence of wrapping, add the trailer to the last of the wrap
		// lines only. This matches what both iTerm and the macOS Terminal does.
		lastRenderedLine.trailer = highlighted.Trailer
	}

	return rendered
}

// Take a rendered line and decorate as needed:
//   - Line number, or leading whitespace for wrapped lines
//   - Scroll left indicator
//   - Scroll right indicator
func (p *Pager) decorateLine(lineNumberToShow *linemetadata.Number, numberPrefixLength int, contents []textstyles.CellWithMetadata) []textstyles.CellWithMetadata {
	width, _ := p.screen.Size()
	if p.WrapLongLines && p.Width > 0 && p.Width < width {
		width = p.Width
	}
	newLine := make([]textstyles.CellWithMetadata, 0, width)
	newLine = append(newLine, createLinePrefix(lineNumberToShow, numberPrefixLength)...)

	// Find the first and last fully visible runes.
	var firstVisibleRuneIndex *int
	lastVisibleRuneIndex := -1
	screenColumn := numberPrefixLength // Zero based
	lastVisibleScreenColumn := p.leftColumnZeroBased + width - 1
	cutOffRuneToTheLeft := false
	cutOffRuneToTheRight := false
	canScrollRight := false
	for i, char := range contents {
		if firstVisibleRuneIndex == nil && screenColumn >= p.leftColumnZeroBased {
			// Found the first fully visible rune. ptr.To(i) gives us a fresh
			// address each iteration, rather than aliasing the loop variable.
			firstVisibleRuneIndex = ptr.To(i)
			if i > 0 && screenColumn > p.leftColumnZeroBased && contents[i-1].Width() > 1 {
				// We had to cut a rune in half at the start
				cutOffRuneToTheLeft = true
			}
		}

		screenReached := firstVisibleRuneIndex != nil
		currentCharRightEdge := screenColumn + char.Width() - 1
		beforeRightEdge := currentCharRightEdge <= lastVisibleScreenColumn
		if screenReached {
			if beforeRightEdge {
				// This rune is fully visible
				lastVisibleRuneIndex = i
			} else {
				// We're just outside the screen on the right
				canScrollRight = true

				currentCharLeftEdge := screenColumn
				if currentCharLeftEdge <= lastVisibleScreenColumn {
					// We have to cut this rune in half
					cutOffRuneToTheRight = true
				}

				// Search done, we're off the right edge
				break
			}
		}

		screenColumn += char.Width()
	}

	// Prepend a space if we had to cut a rune in half at the start
	if cutOffRuneToTheLeft {
		newLine = append([]textstyles.CellWithMetadata{{Rune: ' ', Style: p.ScrollLeftHint.Style}}, newLine...)
	}

	// Add the visible runes
	if firstVisibleRuneIndex != nil {
		newLine = append(newLine, contents[*firstVisibleRuneIndex:lastVisibleRuneIndex+1]...)
	}

	// Append a space if we had to cut a rune in half at the end
	if cutOffRuneToTheRight {
		newLine = append(newLine, textstyles.CellWithMetadata{Rune: ' ', Style: p.ScrollRightHint.Style})
	}

	// Add scroll left indicator
	canScrollLeft := p.leftColumnZeroBased > 0
	if canScrollLeft && len(contents) > 0 {
		if len(newLine) == 0 {
			// Make room for the scroll left indicator
			newLine = make([]textstyles.CellWithMetadata, 1)
		}

		if newLine[0].Width() > 1 {
			// Replace the first rune with two spaces so we can replace the
			// leftmost cell with a scroll left indicator. First, convert to one
			// space...
			newLine[0] = textstyles.CellWithMetadata{Rune: ' ', Style: p.ScrollLeftHint.Style}
			// ...then prepend another space:
			newLine = append([]textstyles.CellWithMetadata{{Rune: ' ', Style: p.ScrollLeftHint.Style}}, newLine...)

			// Prepending ref: https://stackoverflow.com/a/53737602/473672
		}

		// Set can-scroll-left marker
		newLine[0] = p.ScrollLeftHint
	}

	// Add scroll right indicator
	if canScrollRight {
		if newLine[len(newLine)-1].Width() > 1 {
			// Replace the last rune with two spaces so we can replace the
			// rightmost cell with a scroll right indicator. First, convert to one
			// space...
			newLine[len(newLine)-1] = textstyles.CellWithMetadata{Rune: ' ', Style: p.ScrollRightHint.Style}
			// ...then append another space:
			newLine = append(newLine, textstyles.CellWithMetadata{Rune: ' ', Style: p.ScrollRightHint.Style})
		}

		newLine[len(newLine)-1] = p.ScrollRightHint
	}

	return newLine
}

// Generate a line number prefix of the given length.
//
// Can be empty or all-whitespace depending on parameters.
func createLinePrefix(lineNumber *linemetadata.Number, numberPrefixLength int) []textstyles.CellWithMetadata {
	if numberPrefixLength == 0 {
		return []textstyles.CellWithMetadata{}
	}

	lineNumberPrefix := make([]textstyles.CellWithMetadata, 0, numberPrefixLength)
	if lineNumber == nil {
		for len(lineNumberPrefix) < numberPrefixLength {
			lineNumberPrefix = append(lineNumberPrefix, textstyles.CellWithMetadata{Rune: ' '})
		}
		return lineNumberPrefix
	}

	lineNumberString := fmt.Sprintf("%*s ", numberPrefixLength-1, lineNumber.Format())
	if len(lineNumberString) > numberPrefixLength {
		panic(fmt.Errorf(
			"lineNumberString <%s> longer than numberPrefixLength %d",
			lineNumberString, numberPrefixLength))
	}

	for column, digit := range lineNumberString {
		if column >= numberPrefixLength {
			break
		}

		lineNumberPrefix = append(lineNumberPrefix, textstyles.CellWithMetadata{Rune: digit, Style: theme.lineNumbers})
	}

	return lineNumberPrefix
}
