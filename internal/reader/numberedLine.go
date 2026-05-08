package reader

import (
	"github.com/rivo/uniseg"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/twin"
)

type NumberedLine struct {
	Index  linemetadata.Index
	Number linemetadata.Number
	Line   *Line
}

func (nl *NumberedLine) Plain() string {
	return nl.Line.Plain(nl.Index)
}

// maxTokensCount: at most this many tokens will be included in the result. If
// 0, do all runes. For BenchmarkRenderHugeLine() performance.
func (nl *NumberedLine) HighlightedTokens(plainTextStyle twin.Style,
	searchHitStyle twin.Style, search search.Search,
	maxTokensCount int) (textstyles.StyledRunesWithTrailer, search.MatchRanges) {
	return nl.Line.HighlightedTokens(plainTextStyle, searchHitStyle, search, nl.Index, maxTokensCount)
}

func (nl *NumberedLine) DisplayWidth() int {
	width := 0
	for _, r := range nl.Plain() {
		width += uniseg.StringWidth(string(r))
	}
	return width
}
