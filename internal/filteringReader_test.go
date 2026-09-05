package internal

import (
	"os"
	"strings"
	"testing"

	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

func TestOneLine(t *testing.T) {
	pager := Pager{
		screen: twin.NewFakeScreen(20, 10),

		readers: []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "hej")},
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
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

		readers: []*reader.ReaderImpl{reader.NewFromTextForTesting("test", "")},
	}
	pager.filteringReader = FilteringReader{
		BackingReader: pager.readers[pager.currentReader],
		Filter:        &pager.filter,
	}

	pager.filter = search.For("x")
	rendered := pager.renderLines()
	assert.Equal(t, len(rendered.lines), 0)
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
		Filter:        new(search.For("Periodic")),
	}
	fr.rebuildCache() // warmup

	b.ReportAllocs()
	for b.Loop() {
		fr.rebuildCache()
	}
}
