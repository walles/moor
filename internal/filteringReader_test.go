package internal

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
)

func TestOneLine(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(20, 10),

		views: []readerView{{reader: reader.NewFromTextForTesting("test", "hej")}},
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.views[pager.currentView].reader,
		Filter:        &pager.filter,
	}

	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 1)

	pager.filter = search.For("x")
	rendered = pager.renderLines()
	assert.Equal(t, len(rendered.lines), 0)
}

func TestZeroLines(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(20, 10),

		views: []readerView{{reader: reader.NewFromTextForTesting("test", "")}},
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.views[pager.currentView].reader,
		Filter:        &pager.filter,
	}

	pager.filter = search.For("x")
	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 0)
}

// When more bytes arrive for a line without a trailing newline, that line grows
// while the line count stays the same. Filtering must notice that.
func TestFilterSeesGrowingLastLine(t *testing.T) {
	pipeReader, pipeWriter := io.Pipe()
	t.Cleanup(func() { _ = pipeWriter.Close() })

	// The pipe is unbuffered and NewFromStream reads from the stream before
	// returning, so this write has to happen concurrently with that call.
	firstWrite := make(chan error, 1)
	go func() {
		_, err := pipeWriter.Write([]byte("hello"))
		firstWrite <- err
	}()

	backing, err := reader.NewFromStream("test", pipeReader, nil, reader.ReaderOptions{Style: &chroma.Style{}})
	assert.NilError(t, err)
	defer backing.Close()
	assert.NilError(t, <-firstWrite)
	waitForLastLine(t, backing, "hello")

	testMe := FilteringReader{
		BackingReader: backing,
		Filter:        ptr.To(search.For("world")),
	}
	assert.Equal(t, testMe.GetLineCount(), 0)

	_, err = pipeWriter.Write([]byte(" world"))
	assert.NilError(t, err)
	waitForLastLine(t, backing, "hello world")
	assert.Equal(t, backing.GetLineCount(), 1)

	assert.Equal(t, testMe.GetLineCount(), 1, "Grown line now matches the filter")
	line := testMe.GetLine(linemetadata.Index{})
	assert.Equal(t, line.Plain(), "hello world")
}

// Clears the filter the first time its line count is checked after arming,
// like another goroutine would if it cleared the filter at the wrong moment.
type filterClearingReader struct {
	reader.Reader
	filter *search.Search
	armed  bool
}

func (r *filterClearingReader) GetLineCount() int {
	count := r.Reader.GetLineCount()
	if !r.armed {
		return count
	}

	r.armed = false
	*r.filter = search.Search{}
	return count + 1 // Make the cache look stale
}

// Clearing the filter while a filtered read is in progress must not crash.
func TestFilterClearedDuringRead(t *testing.T) {
	filter := search.For("a")
	backing := &filterClearingReader{
		Reader: reader.NewFromTextForTesting("test", "a\nb\na\n"),
		filter: &filter,
	}
	testMe := FilteringReader{
		BackingReader: backing,
		Filter:        &filter,
	}
	assert.Equal(t, testMe.GetLineCount(), 2)

	backing.armed = true
	testMe.GetLineCount() // Either count is fine, as long as we don't crash

	assert.Equal(t, testMe.GetLineCount(), 3, "Filter is cleared now")
}

func waitForLastLine(t *testing.T, r reader.Reader, want string) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		count := r.GetLineCount()
		if count > 0 {
			line := r.GetLine(linemetadata.IndexFromZeroBased(count - 1))
			if line.Plain() == want {
				return
			}
		}

		assert.Assert(t, time.Now().Before(deadline), "timed out waiting for last line %q", want)
		time.Sleep(time.Millisecond)
	}
}

// Micro benchmark for rebuildCache (the main computation in filteringReader.go)
func BenchmarkFilterHugeFile(b *testing.B) {
	// The file packets_repeat.log is 18 kB and 90 lines long (with ANSI colors).
	// To reach 300 MB, it will be repeated ~ 16000 times, resulting in a file with 1.44 M lines.
	f, err := os.ReadFile("../sample-files/packets_repeat.log")
	assert.NilError(b, err)
	reqd_bytes := 300_000_000 // 300MB
	text := strings.Repeat(string(f), reqd_bytes/len(f))
	b.SetBytes(int64(len(text)))

	input := reader.NewFromTextForTesting("BenchmarkFilterHugeFile()", text)
	assert.NilError(b, input.Wait())

	fr := FilteringReader{
		BackingReader: input,
		Filter:        ptr.To(search.For("Periodic")),
	}
	fr.rebuildCache(*fr.Filter) // warmup

	b.ReportAllocs()
	for b.Loop() {
		fr.rebuildCache(*fr.Filter)
	}
}

// Repro for https://github.com/walles/moor/issues/483
func TestSearchFilteredLines(t *testing.T) {
	lines := []string{}
	for i := 1; i <= 1000; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	backingReader := reader.NewFromTextForTesting("TestSearchFilteredLines", strings.Join(lines, "\n"))
	assert.NilError(t, backingReader.Wait())

	filter := search.For("line [1-5]")
	filteringReader := &FilteringReader{
		BackingReader: backingReader,
		Filter:        &filter,
	}

	query := search.For("8")

	// Find all hits the slow way
	expected := []int{}
	for i := 0; ; i++ {
		line := filteringReader.GetLine(linemetadata.IndexFromZeroBased(i))
		if line == nil {
			break
		}
		if query.Matches(line.Plain()) {
			expected = append(expected, line.Index.Index())
		}
	}
	assert.Assert(t, len(expected) > 10)

	// Step through the hits the way pressing "n" does
	actual := []int{}
	position := linemetadata.Index{}
	for {
		hit := FindFirstHit(filteringReader, query, position, nil, SearchDirectionForward)
		if hit == nil {
			break
		}
		actual = append(actual, hit.Index())
		position = hit.NonWrappingAdd(1)
	}

	assert.DeepEqual(t, expected, actual)
}
