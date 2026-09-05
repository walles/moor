package reader

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/go-enry/go-enry/v2"
	log "github.com/sirupsen/logrus"
	"github.com/walles/twin"

	"github.com/walles/moor/v2/internal/ptr"
	"github.com/walles/moor/v2/internal/textstyles"
)

// Read and highlight some text using Chroma:
// https://github.com/alecthomas/chroma
//
// If lexer is nil no highlighting will be performed.
//
// Returns nil with no error if highlighting would be a no-op, which includes the
// case where every visible character would end up looking the same.
func Highlight(text string, style chroma.Style, formatter chroma.Formatter, lexer chroma.Lexer) (*string, error) {
	if lexer == nil {
		// No highlighter available for this file type
		return nil, nil
	}

	// Chroma offers nothing better to test against: every lexer built from its
	// XML files is a *chroma.RegexLexer. Matching the canonical name at least
	// covers the aliases too, "text" and "no-highlight" among them.
	if lexer.Config().Name == "plaintext" {
		// Using this highlighter would paint everything in Chroma's plain text
		// color, changing the look of input that came with escape sequences of
		// its own. See TestHighlightPlaintextIsNoop().
		return nil, nil
	}

	// NOTE: We used to do...
	//
	//   lexer = chroma.Coalesce(lexer)
	//
	// ... here, but with Chroma 2.12.0 that resulted in this problem:
	// https://github.com/walles/moor/issues/236#issuecomment-2282677792
	//
	// So let's not do that anymore.

	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		return nil, err
	}

	var stringBuffer bytes.Buffer
	err = formatter.Format(&stringBuffer, &style, iterator)
	if err != nil {
		return nil, err
	}

	highlighted := stringBuffer.String()
	if isUniformlyStyled(highlighted) {
		return nil, nil
	}

	// If buffer ends with SGR Reset ("<ESC>[0m"), remove it. Chroma sometimes
	// (always?) puts one of those by itself on the last line, making us believe
	// there is one line too many.
	sgrReset := "\x1b[0m"

	return ptr.To(strings.TrimSuffix(highlighted, sgrReset)), nil
}

// Reports whether every visible character of some highlighted text looks the
// same, making the highlighting pointless.
//
// Whitespace shows nothing but its background color, so only backgrounds count
// for whitespace.
func isUniformlyStyled(highlighted string) bool {
	var initialBackground *twin.Color
	var firstVisibleStyle *twin.Style

	for line := range strings.SplitSeq(highlighted, "\n") {
		// Line by line because that's how it will be rendered, and moor starts
		// each line from a clean slate. The nil line index is only used for
		// error reporting.
		for _, cell := range textstyles.StyledRunesFromString(twin.StyleDefault, line, nil, 0).StyledRunes {
			background := cell.Style.Background()
			if initialBackground == nil {
				initialBackground = &background
			} else if background != *initialBackground {
				return false
			}

			if unicode.IsSpace(cell.Rune) {
				// The background is all that shows through whitespace, and it matched
				continue
			}

			if firstVisibleStyle == nil {
				firstVisibleStyle = &cell.Style
				continue
			}

			if !cell.Style.Equal(*firstVisibleStyle) {
				return false
			}
		}
	}

	return true
}

// We expect this to be executed in a goroutine
func highlightFromMemory(reader *ReaderImpl, formatter chroma.Formatter, options ReaderOptions) {
	// Is the buffer small enough?
	var byteCount int64
	reader.RLock()
	for _, line := range reader.lines {
		byteCount += int64(len(line.raw))

		if byteCount > MAX_HIGHLIGHT_SIZE {
			log.Info("File too large for highlighting: ", byteCount)
			reader.RUnlock()
			return
		}
	}
	reader.RUnlock()

	text := textAsString(reader, options.ShouldFormat)

	if len(text) == 0 {
		log.Debug("Buffer is empty, not highlighting")
		return
	}

	if options.Lexer == nil && isJsonOrJsonl(text) {
		log.Info("Buffer is valid JSON or JSONL, highlighting as JSON")
		// The Chroma JSON lexer natively supports JSONL as well:
		// https://github.com/alecthomas/chroma/pull/1262
		options.Lexer = lexers.Get("json")
	}

	if options.Lexer == nil {
		language := enry.GetLanguage("", []byte(text))
		if language != "" {
			log.Info("Buffer language detected as " + language)
			options.Lexer = lexers.Get(language)
		}
	}

	if options.Lexer == nil {
		log.Debug("No lexer set, not highlighting")
		return
	}

	if options.Style == nil {
		log.Debug("No style set, not highlighting")
		return
	}

	if formatter == nil {
		log.Debug("No formatter set, not highlighting")
		return
	}

	highlighted, err := Highlight(text, *options.Style, formatter, options.Lexer)
	if err != nil {
		log.Warn("Highlighting failed: ", err)
		return
	}

	if highlighted == nil {
		// No highlighting would be done, never mind
		return
	}

	reader.setText(*highlighted)
}

func textAsString(reader *ReaderImpl, shouldFormat bool) string {
	reader.RLock()

	text := []byte{}
	for _, line := range reader.lines {
		text = append(text, line.raw...)
		text = append(text, '\n')
	}
	reader.RUnlock()

	var jsonData any
	err := json.Unmarshal(text, &jsonData)
	if err != nil {
		// Not JSON, return the text as-is
		return string(text)
	}

	if !shouldFormat {
		log.Info("Try the --reformat flag for automatic JSON reformatting")
		return string(text)
	}

	// Pretty print the JSON
	prettyJSON, err := json.MarshalIndent(jsonData, "", "  ")
	if err != nil {
		log.Debug("Failed to pretty print JSON: ", err)
		return string(text)
	}

	log.Debug("Got the --reformat flag, reformatted JSON input")
	return string(prettyJSON)
}

func isJsonOrJsonl(text string) bool {
	if json.Valid([]byte(text)) {
		return true
	}

	// It might be jsonl so we split the first line only.
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) > 0 && len(lines[0]) > 2 && lines[0][0] == '{' {
		return json.Valid([]byte(lines[0]))
	}
	return false
}
