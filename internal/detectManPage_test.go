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

// Man pages formatted with SGR escapes or none at all have no overstrike, but
// they still open with a title line like "LS(1)  General Commands  LS(1)". That
// is what BusyBox man (as shipped with w64devkit) gives us.
//
// Ref: https://github.com/walles/moor/issues/475
func TestDetectManPageByHeader(t *testing.T) {
	log.SetLevel(log.WarnLevel)

	for _, header := range []string{
		"LS(1)                        w64devkit Manual                        LS(1)",
		"\x1b[1mLS(1)\x1b[0m                  General Commands Manual                 \x1b[1mLS(1)\x1b[0m",
		"IBUS EMOJI(7)                    User Commands                    IBUS EMOJI(7)",
		"gdbus()                                                                 gdbus()",
		"buildah-manifest-exists(1)()                       buildah-manifest-exists(1)()",

		// groff truncates long titles, sometimes right up against the middle text
		"libpipew...-loopback(7) Miscellaneous Information Manuallibpipew...-loopback(7)",
	} {
		text := header + "\n\nNAME\n       ls - list directory contents\n"
		pager := pagerForManPageDetection(t, text)
		assert.Check(t, pager.haveLoadedManPage(), "header should be detected: %q", header)
	}
}

func TestDetectManPageIgnoresNonHeaders(t *testing.T) {
	log.SetLevel(log.WarnLevel)

	for _, text := range []string{
		"LS(1)                        w64devkit Manual                        LS(8)\n",
		"printf(1)\n",
	} {
		pager := pagerForManPageDetection(t, text)
		assert.Check(t, !pager.haveLoadedManPage(), "should not be detected: %q", text)
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
