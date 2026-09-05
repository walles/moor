package internal

import (
	"os"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

func TestTwinStyleFromChroma(t *testing.T) {
	// Test that getting exact GenericHeading from base16-snazzy works
	style := twinStyleFromChroma(
		nil,
		styles.Registry["base16-snazzy"],
		&formatters.TTY16m,
		chroma.GenericHeading,
		true,
	)

	assert.Equal(t,
		*style,
		twin.StyleDefault.
			WithAttr(twin.AttrBold).
			WithForeground(twin.NewColor24Bit(0xe2, 0xe4, 0xe5)))
}

func TestSetStyle(t *testing.T) {
	assert.NilError(t, os.Setenv("MOOR_TEST_STYLE", "\x1b[1;31m"))
	style := twin.StyleDefault
	setStyle(&style, "MOOR_TEST_STYLE", nil)

	assert.Equal(t, style, twin.StyleDefault.WithAttr(twin.AttrBold).WithForeground(twin.NewColor16(1)))
}

// Regression test for https://github.com/walles/moor/issues/339
//
// We used to crash doing this.
func TestConfigureHighlighting_No24BitColors(t *testing.T) {
	isolateStyles(t)

	theme.searchHit = twin.StyleDefault.WithForeground(twin.NewColor16(3))
	configureHighlighting(nil, true)
}

func TestLessTermcapMdControlsBothBoldAndHeadings(t *testing.T) {
	isolateStyles(t)

	// Bold red
	assert.NilError(t, os.Setenv("LESS_TERMCAP_md", "\x1b[1;31m"))
	t.Cleanup(func() { _ = os.Unsetenv("LESS_TERMCAP_md") })

	// Also bold red
	expectedStyle := twin.StyleDefault.WithAttr(twin.AttrBold).WithForeground(twin.NewColor16(1))

	// Run the setup sequence
	consumeLessTermcapEnvs(nil, nil, nil)
	styleUI(nil, nil, nil, STATUSBAR_STYLE_INVERSE, false, false)

	assert.Equal(t, textstyles.ManPageBold, expectedStyle)
	assert.Equal(t, textstyles.ManPageHeading, expectedStyle)
}
