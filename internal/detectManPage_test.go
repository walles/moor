package internal

import (
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/reader"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

const overstrikeBold = "b\x08bo\x08ol\x08ld\x08d"

func pagerForManPageDetection(t testing.TB, text string) *Pager {
	input := reader.NewFromTextForTesting(t.Name(), text)
	pager := NewPager(input)
	pager.screen = twin.NewFakeScreen(80, 25)
	assert.NilError(t, input.Wait())

	return pager
}

// A man page starts with a title line and a blank line, so its first
// overstrike-formatted line comes a few lines in. Detection looks far enough
// down to find it.
//
// Measured over 14801 man pages from /usr/share/man and Homebrew: the first
// overstrike is on line 13 at the latest, and looking deeper than that found
// no further man pages.
func TestDetectManPageByContents(t *testing.T) {
	log.SetLevel(log.WarnLevel)

	for _, formattedLine := range []int{0, 2, 3, 9, 13} {
		lines := make([]string, 20)
		for i := range lines {
			lines[i] = "unformatted"
		}
		lines[formattedLine] = overstrikeBold

		pager := pagerForManPageDetection(t, strings.Join(lines, "\n"))
		assert.Assert(t, pager.haveLoadedManPage(),
			"overstrike on line %d should be detected", formattedLine)
	}
}

// Text with no overstrike anywhere is not a man page, however long it is.
func TestDetectManPageIgnoresUnformattedText(t *testing.T) {
	log.SetLevel(log.WarnLevel)

	pager := pagerForManPageDetection(t, strings.Repeat("unformatted\n", 100))
	assert.Assert(t, !pager.haveLoadedManPage())
}

// Man page detection runs on the UI goroutine every time the reader reports
// progress, so it must not scan the whole line. Performance sensitive.
//
// To run:
//
//	go test -bench=BenchmarkDetectManPage -benchmem ./internal/
//
// Ref: https://github.com/walles/moor/issues/358
func BenchmarkDetectManPage(b *testing.B) {
	log.SetLevel(log.WarnLevel)

	const megabytes = 5
	builder := strings.Builder{}
	for builder.Len() < megabytes*1024*1024 {
		builder.WriteString("Romani ite domum. ")
	}
	b.SetBytes(int64(builder.Len()))

	pager := pagerForManPageDetection(b, builder.String())

	pager.haveLoadedManPage() // Warm up

	for b.Loop() {
		pager.haveLoadedManPage()
	}
}
