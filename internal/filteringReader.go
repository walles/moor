package internal

import (
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
)

// Filters lines based on the search query from the pager.

type FilteringReader struct {
	BackingReader reader.Reader

	// This is a reference so that we can track changes to the original pattern,
	// including if it is set to nil.
	Filter *search.Search

	// Protects filteredLinesCache, unfilteredLineCountWhenCaching, and
	// filterPatternWhenCaching.
	lock sync.Mutex

	// nil means no filtering has happened yet
	filteredLinesCache *[]reader.NumberedLine

	// This is what the reader's line count was when we filtered. If the
	// reader's current line count doesn't match, then our cache needs to be
	// rebuilt.
	unfilteredLineCountWhenCaching int

	// This is the pattern that was used when we cached the lines. If it
	// doesn't match the current pattern, then our cache needs to be rebuilt.
	filterWhenCaching search.Search
}

// Please hold the lock when calling this method.
//
// This method requires f.Filter.Active() to be true on entry.
func (f *FilteringReader) rebuildCache() {
	filter := *f.Filter
	if !filter.Active() {
		panic("Rebuilding cache requires an active filter")
	}

	t0 := time.Now()

	cache := make([]reader.NumberedLine, 0)

	// Mark cache base conditions
	f.unfilteredLineCountWhenCaching = f.BackingReader.GetLineCount()
	f.filterWhenCaching = filter

	// Repopulate the cache
	resultIndex := 0

	numLines := f.BackingReader.GetLineCount()
	if numLines == 0 {
		f.filteredLinesCache = &cache
		log.Debugf("Filtered out 0/0 lines in %s", time.Since(t0))
		return
	}

	// This completely avoids mutex locks and race conditions during the concurrent phase, while also preserving order.
	matches := make([]bool, numLines)

	var wg sync.WaitGroup
	numWorkers := min(runtime.GOMAXPROCS(0), numLines)

	// chunk size for each goroutine
	chunkSize := (numLines + numWorkers - 1) / numWorkers

	// Concurrent Filtering Phase
	for i := range numWorkers {
		wg.Add(1)
		go func(workerIndex int) {
			defer wg.Done()

			lineCache := searchLineCache{}

			start := workerIndex * chunkSize
			end := min(start+chunkSize, numLines)

			for j := start; j < end; j++ {
				line := lineCache.GetLine(f.BackingReader, linemetadata.IndexFromZeroBased(j), SearchDirectionForward)
				matches[j] = filter.Matches(line.Line.Plain(line.Index, 0))
			}
		}(i)
	}

	wg.Wait()
	t1 := time.Now()

	// Assemble sequentially to ensure resultIndex increments correctly
	lineCache := searchLineCache{}
	for i := range numLines {
		if matches[i] {
			line := lineCache.GetLine(f.BackingReader, linemetadata.IndexFromZeroBased(i), SearchDirectionForward)
			cache = append(cache, reader.NumberedLine{
				Line:   line.Line,
				Index:  linemetadata.IndexFromZeroBased(resultIndex),
				Number: line.Number,
			})
			resultIndex++
		}
	}

	f.filteredLinesCache = &cache

	log.Debugf("Filtered out %d/%d lines in %s (used %d workers in %s)",
		numLines-len(cache), numLines, time.Since(t0), numWorkers, t1.Sub(t0))
}

func (f *FilteringReader) getAllLines() []reader.NumberedLine {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.filteredLinesCache == nil {
		f.rebuildCache()
		return *f.filteredLinesCache
	}

	if f.unfilteredLineCountWhenCaching != f.BackingReader.GetLineCount() {
		f.rebuildCache()
		return *f.filteredLinesCache
	}

	var currentFilterPattern search.Search
	if (*f).Filter.Active() {
		currentFilterPattern = *f.Filter
	}
	var cacheFilterPattern search.Search
	if f.filterWhenCaching.Active() {
		cacheFilterPattern = f.filterWhenCaching
	}
	if !currentFilterPattern.Equals(cacheFilterPattern) {
		f.rebuildCache()
		return *f.filteredLinesCache
	}

	return *f.filteredLinesCache
}

func (f *FilteringReader) shouldPassThrough() bool {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.Filter == nil || f.Filter.Inactive() {
		// Cache is not needed
		f.filteredLinesCache = nil

		// No filtering, so pass through all
		return true
	}

	return false
}

func (f *FilteringReader) GetLineCount() int {
	if f.shouldPassThrough() {
		return f.BackingReader.GetLineCount()
	}

	return len(f.getAllLines())
}

func (f *FilteringReader) ShouldShowLineCount() bool {
	panic("Unexpected call to FilteringReader.ShouldShowLineCount()")
}

func (f *FilteringReader) GetLine(index linemetadata.Index) *reader.NumberedLine {
	if f.shouldPassThrough() {
		return f.BackingReader.GetLine(index)
	}

	allLines := f.getAllLines()
	if index.Index() < 0 || index.Index() >= len(allLines) {
		return nil
	}
	return &allLines[index.Index()]
}

func (f *FilteringReader) GetLines(firstLine linemetadata.Index, wantedLineCount int) reader.InputLines {
	if f.shouldPassThrough() {
		return f.BackingReader.GetLines(firstLine, wantedLineCount)
	}

	acceptedLines := f.getAllLines()

	if len(acceptedLines) == 0 || wantedLineCount == 0 {
		return reader.InputLines{
			StatusText: f.createStatus(nil),
		}
	}

	lastLine := firstLine.NonWrappingAdd(wantedLineCount - 1)

	// Prevent reading past the end of the available lines
	maxLineIndex := *linemetadata.IndexFromLength(len(acceptedLines))
	if lastLine.IsAfter(maxLineIndex) {
		lastLine = maxLineIndex

		// If one line was requested, then first and last should be exactly the
		// same, and we would get there by adding zero.
		firstLine = lastLine.NonWrappingAdd(1 - wantedLineCount)

		return f.GetLines(firstLine, firstLine.CountLinesTo(lastLine))
	}

	return reader.InputLines{
		Lines:      acceptedLines[firstLine.Index() : firstLine.Index()+wantedLineCount],
		StatusText: f.createStatus(&lastLine),
	}
}

func (f *FilteringReader) GetLinesPreallocated(firstLine linemetadata.Index, resultLines *[]reader.NumberedLine) (string, string) {
	if f.shouldPassThrough() {
		return f.BackingReader.GetLinesPreallocated(firstLine, resultLines)
	}

	lines := f.GetLines(firstLine, cap(*resultLines))
	*resultLines = lines.Lines
	return lines.FilenameText, lines.StatusText
}

// In the general case, this will return a text like this:
// "Filtered: 1234/5678 lines  22%"
func (f *FilteringReader) createStatus(lastLine *linemetadata.Index) string {
	baseCount := f.BackingReader.GetLineCount()
	if baseCount == 0 {
		return "Filtered: No input lines"
	}

	baseCountString := "/" + linemetadata.IndexFromLength(baseCount).Format()
	if !f.BackingReader.ShouldShowLineCount() {
		baseCountString = ""
	}

	if lastLine == nil {
		// 100% because we're showing all 0 lines
		return "Filtered: 0" + baseCountString + " lines  100%"
	}

	acceptedCount := f.GetLineCount()
	acceptedCountString := linemetadata.IndexFromLength(acceptedCount).Format()

	percent := int(math.Floor(100 * float64(lastLine.Index()+1) / float64(acceptedCount)))

	lineString := "line"
	if (len(baseCountString) > 0 && baseCount != 1) || (len(baseCountString) == 0 && acceptedCount != 1) {
		lineString += "s"
	}

	return fmt.Sprintf("Filtered: %s%s %s  %d%%",
		acceptedCountString, baseCountString, lineString, percent)
}

// SetBackingReader switches the underlying reader while holding the lock and
// clears all cached state so that subsequent calls will rebuild using the new
// reader.
//
// This avoids data races that would occur if the entire FilteringReader struct
// (and its mutex) were reassigned concurrently.
func (f *FilteringReader) SetBackingReader(r reader.Reader) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.BackingReader = r

	// Invalidate caches so they will be rebuilt lazily on next access.
	f.filteredLinesCache = nil
	f.unfilteredLineCountWhenCaching = -1
	f.filterWhenCaching = search.Search{}
}
