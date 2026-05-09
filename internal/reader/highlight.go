package reader

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	log "github.com/sirupsen/logrus"
)

// Read and highlight some text using Chroma:
// https://github.com/alecthomas/chroma
//
// If lexer is nil no highlighting will be performed.
//
// Returns nil with no error if highlighting would be a no-op.
func Highlight(text string, style chroma.Style, formatter chroma.Formatter, lexer chroma.Lexer) (*string, error) {
	if lexer == nil {
		// No highlighter available for this file type
		return nil, nil
	}

	// FIXME: Can we test for the lexer implementation class instead? That
	// should be more resilient towards this arbitrary string changing if we
	// upgrade Chroma at some point.
	if lexer.Config().Name == "plaintext" {
		// This highlighter doesn't provide any highlighting, but not doing
		// anything at all is cheaper and simpler, so we do that.
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

	// If buffer ends with SGR Reset ("<ESC>[0m"), remove it. Chroma sometimes
	// (always?) puts one of those by itself on the last line, making us believe
	// there is one line too many.
	sgrReset := "\x1b[0m"
	trimmed := strings.TrimSuffix(highlighted, sgrReset)

	return &trimmed, nil
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
	} else if options.Lexer == nil && isXml(text) {
		log.Info("Buffer is valid XML, highlighting as XML")
		options.Lexer = lexers.Get("xml")
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

func isXml(text string) bool {
	err := xml.Unmarshal([]byte(text), new(any))
	return err == nil
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
