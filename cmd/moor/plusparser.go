package main

import (
	"strconv"
	"strings"

	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/ptr"
)

// Parses arguments starting with '+':
//
// - +1234 sets the initial line number to 1234 (one-based)
// - +/pattern starts at the first line matching the pattern
func parsePlusArgs(args []string) (*linemetadata.Index, *string, []string) {
	remainingArgs := make([]string, 0)
	var targetIndex *linemetadata.Index
	var searchPattern *string

	for _, arg := range args {
		if !strings.HasPrefix(arg, "+") {
			// Not a valid plus argument, keep it
			remainingArgs = append(remainingArgs, arg)
			continue
		}

		withoutPlus := arg[1:]

		parsedIndex := parseLineNumber(withoutPlus)
		if parsedIndex != nil {
			targetIndex = parsedIndex
			continue
		}

		parsedSearchPattern := parseSearchPattern(withoutPlus)
		if parsedSearchPattern != nil {
			searchPattern = parsedSearchPattern
			continue
		}

		// Not a valid plus argument, keep it
		remainingArgs = append(remainingArgs, arg)
	}

	return targetIndex, searchPattern, remainingArgs
}

func parseLineNumber(withoutPlus string) *linemetadata.Index {
	lineNumber, err := strconv.ParseInt(withoutPlus, 10, 32)
	if err != nil {
		return nil
	}
	if lineNumber < 0 {
		return nil
	}

	if lineNumber == 0 {
		// Special case +0 because that's what less does:
		// https://github.com/walles/moor/issues/316
		lineNumber = 1
	}

	return ptr.To(linemetadata.IndexFromOneBased(int(lineNumber)))
}

func parseSearchPattern(withoutPlus string) *string {
	if !strings.HasPrefix(withoutPlus, "/") {
		return nil
	}

	return ptr.To(withoutPlus[1:])
}
