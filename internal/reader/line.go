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
	search search.Search,
	lineIndex linemetadata.Index,
	maxTokensCount int,
) textstyles.StyledRunesWithTrailer {
	matchRanges := search.GetMatchRanges(line.Plain(lineIndex, maxTokensCount))

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
	}
}

func (line *Line) HasManPageFormatting() bool {
	return textstyles.HasManPageFormatting(string(line.raw))
}

// The index is for error reporting. Set withCache to false to simulate a cache
// miss for benchmarking.
//
// maxTokensCount: at most this many runes will be included in the result. If
// 0, do all runes. For BenchmarkRenderHugeLine() performance.
func (line *Line) Plain(index linemetadata.Index, maxTokensCount int) string {
	fromCache := line.plainTextCache.Load()
	if DisablePlainCachingForBenchmarking {
		// Simulate a cache miss for benchmarking
		fromCache = nil
	}
	if fromCache != nil {
		if maxTokensCount > 0 {
			runeCount := 0
			for byteIndex := range *fromCache {
				if runeCount == maxTokensCount {
					return (*fromCache)[:byteIndex]
				}
				runeCount++
			}
		}
		return *fromCache
	}

	plain := textstyles.StripFormatting(string(line.raw), index, maxTokensCount)

	if maxTokensCount == 0 {
		// If this succeeds, all good. If it fails it means some other goroutine
		// populated the cache before us, which is also fine.
		_ = line.plainTextCache.CompareAndSwap(nil, &plain)
	}

	return plain
}
