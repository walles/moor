// This file contains code for scanning input lines for search hits.

package internal

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
)

// Go for at least this many lines per core when searching
const minLinesPerCore = 1000

// Search input lines. Not screen lines!
//
// The `beforePosition` parameter is exclusive, meaning that line will not be
// searched.
//
// For the actual searching, this method will call _findFirstHit() in parallel
// on multiple cores, to help large file search performance.
func FindFirstHit(reader reader.Reader, search search.Search, startPosition linemetadata.Index, beforePosition *linemetadata.Index, direction SearchDirection) *linemetadata.Index {
	var linesCount int
	if direction == SearchDirectionBackward {
		// If the startPosition is zero, that should make the count one
		linesCount = startPosition.Index() + 1
		if beforePosition != nil {
			// Searching from 1 with before set to 0 should make the count 1
			linesCount = startPosition.Index() - beforePosition.Index()
		}
	} else {
		linesCount = reader.GetLineCount() - startPosition.Index()
		if beforePosition != nil {
			// Searching from 1 with before set to 2 should make the count 1
			linesCount = beforePosition.Index() - startPosition.Index()
		}
	}

	// How many cores should we use?
	chunkCount := runtime.NumCPU()

	maxChunkCount := linesCount / minLinesPerCore
	if chunkCount > maxChunkCount {
		chunkCount = maxChunkCount
	}
	if chunkCount < 1 {
		chunkCount = 1
	}
	chunkSize := linesCount / chunkCount

	log.Debugf("Searching %d lines across %d cores with %d lines per core...", linesCount, chunkCount, chunkSize)
	t0 := time.Now()
	defer func() {
		dt := time.Since(t0)
		linesPerSecond := float64(linesCount) / dt.Seconds()
		linesPerSecondS := fmt.Sprintf("%.0f", linesPerSecond)
		if linesPerSecond > 7_000_000_000.0 {
			linesPerSecondS = fmt.Sprintf("%.0fG", linesPerSecond/1000_000_000.0)
		} else if linesPerSecond > 7_000_000.0 {
			linesPerSecondS = fmt.Sprintf("%.0fM", linesPerSecond/1000_000.0)
		} else if linesPerSecond > 7_000.0 {
			linesPerSecondS = fmt.Sprintf("%.0fk", linesPerSecond/1000.0)
		}

		if linesCount > 0 {
			log.Debugf("Searched %d lines in %s at %slines/s or %s/line",
				linesCount,
				dt,
				linesPerSecondS,
				(dt / time.Duration(linesCount)).String())
		} else {
			log.Debugf("Searched %d lines in %s at %slines/s", linesCount, dt, linesPerSecondS)
		}
	}()

	// Each parallel search will start at one of these positions
	searchStarts := make([]linemetadata.Index, chunkCount)
	directionSign := 1
	if direction == SearchDirectionBackward {
		directionSign = -1
	}
	for i := 0; i < chunkCount; i++ {
		searchStarts[i] = startPosition.NonWrappingAdd(i * directionSign * chunkSize)
	}

	// Make a results array, with one result per chunk
	findings := make([]chan *linemetadata.Index, chunkCount)

	// Search all chunks in parallel
	for i, searchStart := range searchStarts {
		// Provide a buffer of 1 so go-routines don't block
		// if FindFirstHit returns early.
		findings[i] = make(chan *linemetadata.Index, 1)

		searchEndIndex := i + 1
		var chunkBefore *linemetadata.Index
		if searchEndIndex < len(searchStarts) {
			chunkBefore = &searchStarts[searchEndIndex]
		} else if beforePosition != nil {
			chunkBefore = beforePosition
		}

		go func(i int, searchStart linemetadata.Index, chunkBefore *linemetadata.Index) {
			defer func() {
				PanicHandler("findFirstHit()/chunkSearch", recover(), debug.Stack())
			}()

			findings[i] <- _findFirstHit(reader, searchStart, search, chunkBefore, direction)
		}(i, searchStart, chunkBefore)
	}

	// Return the first non-nil result
	for _, finding := range findings {
		result := <-finding
		if result != nil {
			return result
		}
	}

	return nil
}

// NOTE: When we search, we do that by looping over the *input lines*, not the
// screen lines. That's why startPosition is an Index rather than a
// scrollPosition.
//
// The `beforePosition` parameter is exclusive, meaning that line will not be
// searched.
//
// This method will run over multiple chunks of the input file in parallel to
// help large file search performance.
func _findFirstHit(reader reader.Reader, startPosition linemetadata.Index, search search.Search, beforePosition *linemetadata.Index, direction SearchDirection) *linemetadata.Index {
	searchPosition := startPosition
	lineCache := searchLineCache{}
	for {
		line := lineCache.GetLine(reader, searchPosition, direction)
		if line == nil {
			// No match, give up
			return nil
		}

		lineText := line.Plain()
		if search.Matches(lineText) {
			return &searchPosition
		}

		if direction == SearchDirectionForward {
			searchPosition = searchPosition.NonWrappingAdd(1)
		} else {
			if (searchPosition == linemetadata.Index{}) {
				// Reached the top without any match, give up
				return nil
			}

			searchPosition = searchPosition.NonWrappingAdd(-1)
		}

		if beforePosition != nil && searchPosition == *beforePosition {
			// No match, give up
			return nil
		}
	}
}
