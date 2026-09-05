package main

import (
	"testing"

	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/twin"
	"gotest.tools/v3/assert"
)

func TestParseScrollHint(t *testing.T) {
	token, err := parseScrollHint("ESC[7m>")
	assert.NilError(t, err)
	assert.Equal(t, token, textstyles.CellWithMetadata{
		Rune:  '>',
		Style: twin.StyleDefault.WithAttr(twin.AttrReverse),
	})
}

func TestUnescapeManPn(t *testing.T) {
	assert.Equal(t, unescapeManPn(`printf(1)`), `printf(1)`)
	assert.Equal(t, unescapeManPn(`xpcservice\.plist(5)`), `xpcservice.plist(5)`)
	assert.Equal(t, unescapeManPn(`foo\:bar\%baz\?qux`), `foo:bar%baz?qux`)
	assert.Equal(t, unescapeManPn(`a\\b`), `a\b`)

	// Trailing backslash or unknown escapes (should just be preserved)
	assert.Equal(t, unescapeManPn(`a\`), `a\`)
	assert.Equal(t, unescapeManPn(`a\b`), `a\b`)
}

func TestPageOneInputFile(t *testing.T) {
	pager, screen, _, formatter, _, err := pagerFromArgs(
		[]string{"", "moor_test.go"},
		func(_ twin.Options) (twin.Screen, error) {
			return twin.NewFakeScreen(80, 24), nil
		},
		false, // stdin is redirected
		false, // stdout is redirected
	)

	assert.NilError(t, err)
	assert.Assert(t, pager != nil)
	assert.Assert(t, screen != nil)
	assert.Assert(t, formatter != nil)
}
