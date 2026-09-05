package reader

import (
	"sync/atomic"
	"unsafe"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/twin"
)

type Line struct {
	raw            []byte
	plainTextCache atomic.Pointer[string] // Use line.Plain() to access this field
}

// The raw bytes as a string. Not copied, so the result aliases the line's
// backing array rather than snapshotting it.
//
// Performance sensitive, see BenchmarkRenderHugeLine().
func (line *Line) rawString() string {
	// Safe because raw is never written once the Line has been published to
	// readers. A line that grows is replaced by a longer Line.
	return unsafe.String(unsafe.SliceData(line.raw), len(line.raw))
}

// Returns a representation of the string split into styled tokens. Any regexp
// matches are highlighted. A nil regexp means no highlighting.
//
// maxCellsCount: at most this many cells will be included in the result. If 0,
// there is no limit. For BenchmarkRenderHugeLine() performance.
func (line *Line) HighlightedTokens(
	plainTextStyle twin.Style,
	searchHitStyle twin.Style,
	activeSearch search.Search,
	lineIndex linemetadata.Index,
	maxCellsCount int,
) textstyles.StyledRunesWithTrailer {
	var matchRanges *search.MatchRanges
	containsSearchHit := false
	if activeSearch.Active() {
		// Only look for matches if there is an active search, since if a line
		// is 250M characters long, line.Plain() can be slow.
		//
		// This makes the UI responsive when showing a huge line.
		plain := line.Plain(lineIndex)

		matchRanges = activeSearch.GetMatchRanges(plain, maxCellsCount)

		// matchRanges covers only the cells we return, but callers use
		// ContainsSearchHit to mark hits anywhere in the line.
		containsSearchHit = !matchRanges.Empty() || activeSearch.Matches(plain)
	}

	fromString := textstyles.StyledRunesFromString(plainTextStyle, line.rawString(), &lineIndex, maxCellsCount)
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
		ContainsSearchHit: containsSearchHit,
	}
}

func (line *Line) HasManPageFormatting() bool {
	return textstyles.HasManPageFormatting(line.rawString())
}

// The index is for error reporting. Set DisablePlainCachingForBenchmarking to
// false to simulate a cache miss for benchmarking.
func (line *Line) Plain(index linemetadata.Index) string {
	fromCache := line.plainTextCache.Load()
	if DisablePlainCachingForBenchmarking {
		// Simulate a cache miss for benchmarking
		fromCache = nil
	}
	if fromCache != nil {
		return *fromCache
	}

	plain := textstyles.StripFormatting(line.rawString(), index)

	// If this succeeds, all good. If it fails it means some other goroutine
	// populated the cache before us, which is also fine.
	_ = line.plainTextCache.CompareAndSwap(nil, &plain)

	return plain
}
