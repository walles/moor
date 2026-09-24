package internal

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/walles/moor/v2/internal/textstyles"
)

// colorlessChromaStyle is a Chroma style without colors. With it, text keeps the
// terminal's default colors, and man page styles match moor's built-in ones.
var colorlessChromaStyle = chroma.MustNewStyle("colorless", chroma.StyleEntries{
	chroma.GenericStrong:    "bold",
	chroma.GenericHeading:   "bold",
	chroma.GenericUnderline: "underline",
})

// isolateStyles gives this test the default styles, and restores them when the
// test is done.
//
// Styling lives in package level variables, so without this a test that styles
// the UI changes what every later test in the package sees. Call it from any
// test that does styling, either directly or by starting a pager.
func isolateStyles(t *testing.T) {
	reset := func() {
		theme = defaultUiStyles()
		textstyles.ResetManPageStyles()
	}

	reset()
	t.Cleanup(reset)
}
