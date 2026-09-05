package textstyles

import (
	"unicode"

	"github.com/walles/twin"
)

func manPageHeadingFromString(s string) *StyledRunesWithTrailer {
	// For great performance, first check the string without allocating any
	// memory.
	if !parseManPageHeading(s, func(_ CellWithMetadata) {}) {
		return nil
	}

	cells := make([]CellWithMetadata, 0, len(s)/2)
	ok := parseManPageHeading(s, func(cell CellWithMetadata) {
		cells = append(cells, cell)
	})
	if !ok {
		panic("man page heading state changed")
	}

	return &StyledRunesWithTrailer{
		StyledRunes: cells,
		Trailer:     twin.StyleDefault,
	}
}

// Reports back one cell at a time. Returns true if the entire string was a man
// page heading.
//
// If it was not, false will be returned and the cell reporting will be
// interrupted.
//
// A man page heading is all caps. Also, each character is encoded as
// char+backspace+char, where both chars need to be the same. Whitespace is an
// exception, they can be not bold.
func parseManPageHeading(s string, reportStyledRune func(CellWithMetadata)) bool {
	if len(s) < 3 {
		// We don't want to match empty strings. Also, strings of length 1 and 2
		// cannot be man page headings since "char+backspace+char" is 3 bytes.
		return false
	}

	type stateT int
	const (
		stateExpectingFirstChar stateT = iota
		stateExpectingBackspace
		stateExpectingSecondChar
	)

	state := stateExpectingFirstChar
	var firstChar rune
	lapCounter := -1
	for _, char := range s {
		lapCounter++

		switch state {
		case stateExpectingFirstChar:
			if lapCounter == 0 && unicode.IsSpace(char) {
				// Headings do not start with whitespace
				return false
			}

			if char == '\b' {
				// Starting with backspace is an error
				return false
			}
			firstChar = char
			state = stateExpectingBackspace

		case stateExpectingBackspace:
			if char == '\b' {
				state = stateExpectingSecondChar
				continue
			}

			if unicode.IsSpace(firstChar) {
				// Whitespace is an exception, it can be not bold
				reportStyledRune(CellWithMetadata{Rune: firstChar, Style: ManPageHeading})

				// Assume what we got was a new first char
				firstChar = char
				state = stateExpectingBackspace
				continue
			}

			// No backspace and no previous-was-whitespace, this is an error
			return false

		case stateExpectingSecondChar:
			if char == '\b' {
				// Ending with backspace is an error
				return false
			}

			if char != firstChar {
				// Different first and last char is an error
				return false
			}

			if unicode.IsLetter(char) && !unicode.IsUpper(char) {
				// Not ALL CAPS => Not a heading
				return false
			}

			reportStyledRune(CellWithMetadata{Rune: char, Style: ManPageHeading})
			state = stateExpectingFirstChar

		default:
			panic("Unknown state")
		}
	}

	return state == stateExpectingFirstChar
}
