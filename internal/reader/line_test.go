package reader

import (
	"testing"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

func TestHighlightedTokensWithManPageHeading(t *testing.T) {
	// Set a marker style we can recognize and test for
	textstyles.ManPageHeading = twin.StyleDefault.WithForeground(twin.NewColor16(2))

	headingText := "JOHAN"

	manPageHeading := ""
	for _, char := range headingText {
		manPageHeading += string(char) + "\b" + string(char)
	}

	highlighted := textstyles.StyledRunesFromString(twin.StyleDefault, manPageHeading, nil, 0).StyledRunes

	assert.Equal(t, len(highlighted), len(headingText))
	for i, cell := range highlighted {
		assert.Equal(t, cell.Rune, rune(headingText[i]))
		assert.Equal(t, cell.Style, textstyles.ManPageHeading)
	}
}

// Verify that a multi-rune search hit spanning a simulated wrap boundary
// propagates the search-hit markers to both sub-lines.
//
// We don't call the actual wrapLine() here (different package + unexported);
// instead we simulate a wrap by slicing at a chosen width. All runes are
// single-width here so rune index == screen column.
func TestSearchHitSpanningWrapBoundary(t *testing.T) {
	// Arrange: a line where the search hit crosses index 5
	line := NewFromTextForTesting("TestSearchHitSpanningWrapBoundary", "0123456789").GetLine(linemetadata.Index{}).Line
	searchHitStyle := twin.StyleDefault.WithForeground(twin.NewColor16(3))

	// Match runs from indices 3..8 inclusive ("345678")
	highlighted := line.HighlightedTokens(twin.StyleDefault, searchHitStyle, search.For("345678"), linemetadata.Index{}, 0)

	// Sanity: overall line reports having a search hit
	assert.Assert(t, highlighted.ContainsSearchHit, "Expected overall line to contain search hit")

	wrapWidth := 5 // Split after index 4
	if len(highlighted.StyledRunes) <= wrapWidth+1 {
		t.Fatalf("Unexpected rune count %d, need > %d", len(highlighted.StyledRunes), wrapWidth+1)
	}

	first := textstyles.CellWithMetadataSlice(highlighted.StyledRunes[:wrapWidth])
	second := textstyles.CellWithMetadataSlice(highlighted.StyledRunes[wrapWidth:])

	// Assert: both wrapped parts contain search hit cells (continuation preserved)
	assert.Assert(t, first.ContainsSearchHit(), "First part should contain start of search hit")
	assert.Assert(t, second.ContainsSearchHit(), "Second part should contain continuation of search hit spanning wrap")

	// Additionally ensure styling applied to all hit cells (foreground color matches)
	for _, cell := range append(first, second...) {
		if cell.IsSearchHit && !cell.Style.Equal(searchHitStyle) {
			t.Fatalf("Search hit cell %#v does not have expected searchHitStyle", cell)
		}
	}
}
