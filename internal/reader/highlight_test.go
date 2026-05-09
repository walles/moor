package reader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/walles/moor/v2/internal/linemetadata"
	"gotest.tools/v3/assert"
)

func TestFormatJson(t *testing.T) {
	// Note the space after "key" to verify formatting actually happens
	jsonStream := strings.NewReader(`{"key" :"value"}`)
	testMe, err := NewFromStream(
		"JSON test",
		jsonStream,
		formatters.TTY,
		ReaderOptions{
			Style:        styles.Get("native"),
			ShouldFormat: true,
		})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())

	lines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, lines.Lines[0].Plain(), "{")
	assert.Equal(t, lines.Lines[1].Plain(), `  "key": "value"`)
	assert.Equal(t, lines.Lines[2].Plain(), "}")
	assert.Equal(t, len(lines.Lines), 3)
}

func TestFormatJsonArray(t *testing.T) {
	// Note the space after "key" to verify formatting actually happens
	jsonStream := strings.NewReader(`[{"key" :"value"}]`)
	testMe, err := NewFromStream(
		"JSON test",
		jsonStream,
		formatters.TTY,
		ReaderOptions{
			Style:        styles.Get("native"),
			ShouldFormat: true,
		})
	assert.NilError(t, err)

	assert.NilError(t, testMe.Wait())

	lines := testMe.GetLines(linemetadata.Index{}, 10)
	assert.Equal(t, lines.Lines[0].Plain(), "[")
	assert.Equal(t, lines.Lines[1].Plain(), "  {")
	assert.Equal(t, lines.Lines[2].Plain(), `    "key": "value"`)
	assert.Equal(t, lines.Lines[3].Plain(), "  }")
	assert.Equal(t, lines.Lines[4].Plain(), "]")
	assert.Equal(t, len(lines.Lines), 5)
}

func TestIsJsonOrJsonl(t *testing.T) {
	// Standard JSON
	assert.Assert(t, isJsonOrJsonl(`{"hello": "world"}`))

	// JSONL sample file
	jsonlPath := filepath.Join("..", "..", "sample-files", "jsonl.jsonl")
	jsonlBytes, err := os.ReadFile(jsonlPath)
	assert.NilError(t, err)

	assert.Assert(t, isJsonOrJsonl(string(jsonlBytes)))
}
