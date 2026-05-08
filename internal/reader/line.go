package reader

import (
	"sync/atomic"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/twin"
)

type Line struct {
	raw            []byte
	plainTextCache atomic.Pointer[string] // Use line.Plain() to access this field
}

// Returns a representation of the string split into styled tokens. Any regexp
// matches are highlighted. A nil regexp means no highlighting.
//
// maxTokensCount: at most this many tokens will be included in the result. If
// 0, do all runes. For BenchmarkRenderHugeLine() performance.
func (line *Line) HighlightedTokens(
	plainTextStyle twin.Style,
	searchHitStyle twin.Style,
	search_term search.Search,
	lineIndex linemetadata.Index,
	maxTokensCount int,
) (textstyles.StyledRunesWithTrailer, search.MatchRanges) {
	matchRanges := search_term.GetMatchRanges(line.Plain(lineIndex), lineIndex)

	fromString := textstyles.StyledRunesFromString(plainTextStyle, string(line.raw), &lineIndex, maxTokensCount)
	returnRunes := make([]textstyles.CellWithMetadata, 0, len(fromString.StyledRunes))
	lastWasSearchHit := false
	for _, token := range fromString.StyledRunes {
		style := token.Style
		searchHit := matchRanges.InRange(len(returnRunes))
		if searchHit {
			// Highlight the search hit
			style = searchHitStyle
		}

		returnRunes = append(returnRunes, textstyles.CellWithMetadata{
			Rune:            token.Rune,
			Style:           style,
			IsSearchHit:     searchHit,
			StartsSearchHit: searchHit && !lastWasSearchHit,
		})
		lastWasSearchHit = searchHit
	}

	return textstyles.StyledRunesWithTrailer{
		StyledRunes:       returnRunes,
		Trailer:           fromString.Trailer,
		ContainsSearchHit: !matchRanges.Empty(),
	}, matchRanges
}

func (line *Line) HasManPageFormatting() bool {
	return textstyles.HasManPageFormatting(string(line.raw))
}

// The index is for error reporting. Set withCache to false to simulate a cache
// miss for benchmarking.
func (line *Line) Plain(index linemetadata.Index) string {
	fromCache := line.plainTextCache.Load()
	if DisablePlainCachingForBenchmarking {
		// Simulate a cache miss for benchmarking
		fromCache = nil
	}
	if fromCache != nil {
		return *fromCache
	}

	plain := textstyles.StripFormatting(string(line.raw), index)

	// If this succeeds, all good. If it fails it means some other goroutine
	// populated the cache before us, which is also fine.
	_ = line.plainTextCache.CompareAndSwap(nil, &plain)

	return plain
}
