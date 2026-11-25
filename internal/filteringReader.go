package internal

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
)

// Filters lines based on the search query from the pager.

type FilteringReader struct {
	BackingReader reader.Reader

	// This is a reference so that we can track changes to the original pattern,
	// including if it is set to nil.
	Filter *search.Search

	// Protects filteredLinesCache, unfilteredLineCountWhenCaching,
	// lastLineWhenCaching and filterWhenCaching.
	lock sync.Mutex

	// nil means no filtering has happened yet
	filteredLinesCache *[]reader.NumberedLine

	// This is what the reader's line count was when we filtered. If the
	// reader's current line count doesn't match, then our cache needs to be
	// rebuilt.
	unfilteredLineCountWhenCaching int

	// The reader's last line when we filtered. Bytes arriving for a last line
	// without a trailing newline replace it with a longer one, without changing
	// the line count. If this doesn't match, our cache needs to be rebuilt.
	lastLineWhenCaching *reader.Line

	// This is the pattern that was used when we cached the lines. If it
	// doesn't match the current pattern, then our cache needs to be rebuilt.
	filterWhenCaching search.Search
}

// Please hold the lock when calling this method.
//
// filter must be active.
func (f *FilteringReader) rebuildCache(filter search.Search) {
	t0 := time.Now()

	cache := make([]reader.NumberedLine, 0)

	// Mark cache base conditions. Get the last line before filtering, so that
	// if it grows while we filter, we'll notice next time.
	numLines := f.BackingReader.GetLineCount()
	f.unfilteredLineCountWhenCaching = numLines
	f.lastLineWhenCaching = lastLine(f.BackingReader, numLines)
	f.filterWhenCaching = filter

	// Repopulate the cache
	resultIndex := 0

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
				matches[j] = filter.Matches(line.Line.Plain(line.Index))
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

// The lines that pass the filter, or ok=false if there is no active filter and
// the caller should pass through to the backing reader.
//
// Deciding that and filtering under one lock, with one copy of the filter,
// means that someone clearing the filter can't get in between.
func (f *FilteringReader) filteredLines() (lines []reader.NumberedLine, ok bool) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.Filter == nil || f.Filter.Inactive() {
		// Cache is not needed
		f.filteredLinesCache = nil

		// No filtering, so pass through all
		return nil, false
	}
	filter := *f.Filter

	if f.filteredLinesCache == nil ||
		f.unfilteredLineCountWhenCaching != f.BackingReader.GetLineCount() ||
		f.lastLineWhenCaching != lastLine(f.BackingReader, f.unfilteredLineCountWhenCaching) ||
		!filter.Equals(f.filterWhenCaching) {
		f.rebuildCache(filter)
	}

	return *f.filteredLinesCache, true
}

// Find the index of the line with this unfiltered line number.
//
// If the filter hides that line, return the closest shown line above it, and
// hidden true. If no shown line is at or above it, return nil and hidden true.
//
// Without a filter, a line past the end (the file shrank) gives the last line.
// With no lines at all, return nil.
func (f *FilteringReader) findLine(number linemetadata.Number) (index *linemetadata.Index, hidden bool) {
	allLines, ok := f.filteredLines()
	if !ok {
		lineCount := f.BackingReader.GetLineCount()
		if lineCount == 0 {
			return nil, false
		}

		// The line can be gone if the file got shorter when reloading
		return ptr.To(linemetadata.IndexFromZeroBased(min(number.AsZeroBased(), lineCount-1))), false
	}

	// Count the lines at or before the one we want
	count := sort.Search(len(allLines), func(i int) bool {
		return allLines[i].Number.IsAfter(number)
	})
	if count == 0 {
		return nil, true
	}

	found := allLines[count-1]
	return ptr.To(found.Index), found.Number != number
}

func (f *FilteringReader) GetLineCount() int {
	allLines, ok := f.filteredLines()
	if !ok {
		return f.BackingReader.GetLineCount()
	}

	return len(allLines)
}

func (f *FilteringReader) ShouldShowLineCount() bool {
	panic("Unexpected call to FilteringReader.ShouldShowLineCount()")
}

// Filtering is done on demand, so all filtered lines are available once
// the backing reader has read all of its lines
func (f *FilteringReader) ReadingDone() bool {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.BackingReader.ReadingDone()
}

func (f *FilteringReader) GetLine(index linemetadata.Index) *reader.NumberedLine {
	allLines, ok := f.filteredLines()
	if !ok {
		return f.BackingReader.GetLine(index)
	}

	if index.Index() < 0 || index.Index() >= len(allLines) {
		return nil
	}
	return &allLines[index.Index()]
}

func (f *FilteringReader) GetLines(firstLine linemetadata.Index, wantedLineCount int) reader.InputLines {
	acceptedLines, ok := f.filteredLines()
	if !ok {
		return f.BackingReader.GetLines(firstLine, wantedLineCount)
	}

	if len(acceptedLines) == 0 || wantedLineCount == 0 {
		return reader.InputLines{Status: f.createStatus(nil, len(acceptedLines))}
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
		Lines:  acceptedLines[firstLine.Index() : firstLine.Index()+wantedLineCount],
		Status: f.createStatus(&lastLine, len(acceptedLines)),
	}
}

func (f *FilteringReader) GetLinesPreallocated(firstLine linemetadata.Index, resultLines *[]reader.NumberedLine) reader.Status {
	if _, ok := f.filteredLines(); !ok {
		return f.BackingReader.GetLinesPreallocated(firstLine, resultLines)
	}

	// Copy into the caller's slice. Callers rely on its capacity staying the
	// same, and they would write into our filtered lines cache if we gave them
	// a slice of it.
	lines := f.GetLines(firstLine, cap(*resultLines))
	*resultLines = append((*resultLines)[:0], lines.Lines...)
	return lines.Status
}

// In the general case, this will return a text like this:
// "Filtered: 1234/5678 lines  22%"
func (f *FilteringReader) createStatus(lastLine *linemetadata.Index, acceptedCount int) reader.Status {
	baseCount := f.BackingReader.GetLineCount()
	if baseCount == 0 {
		return reader.Status{StatusText: "Filtered: No input lines"}
	}

	baseCountString := "/" + linemetadata.IndexFromLength(baseCount).Format()
	if !f.BackingReader.ShouldShowLineCount() {
		baseCountString = ""
	}

	if lastLine == nil {
		// 100% because we're showing all 0 lines
		return reader.Status{StatusText: "Filtered: 0" + baseCountString + " lines  100%"}
	}

	acceptedCountString := linemetadata.IndexFromLength(acceptedCount).Format()

	percent := int(math.Floor(100 * float64(lastLine.Index()+1) / float64(acceptedCount)))

	lineString := "line"
	if (len(baseCountString) > 0 && baseCount != 1) || (len(baseCountString) == 0 && acceptedCount != 1) {
		lineString += "s"
	}

	return reader.Status{
		StatusText: fmt.Sprintf("Filtered: %s%s %s  %d%%",
			acceptedCountString, baseCountString, lineString, percent),
	}
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
	f.lastLineWhenCaching = nil
	f.filterWhenCaching = search.Search{}
}

// The last of the first lineCount lines in r, or nil if lineCount is 0. Unlike
// GetLine(), this doesn't make the reader read further ahead.
func lastLine(r reader.Reader, lineCount int) *reader.Line {
	index := linemetadata.IndexFromLength(lineCount)
	if index == nil {
		return nil
	}

	lines := r.GetLines(*index, 1).Lines
	if len(lines) == 0 {
		return nil
	}
	return lines[0].Line
}
