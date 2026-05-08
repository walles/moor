package search

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/charlievieth/strcase"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/util"
)

type Search struct {
	findMe string

	// If this is false it means the input has to be interpreted as a regexp.
	isSubstringSearch bool

	hasUppercase bool

	pattern *regexp.Regexp

	Cache *SearchCache
}

func (search Search) Equals(other Search) bool {
	return search.findMe == other.findMe
}

func (search Search) String() string {
	return search.findMe
}

func For(s string) Search {
	search := Search{}
	search.For(s)
	return search
}

func (search *Search) For(s string) *Search {
	search.findMe = s
	if s == "" {
		// No search
		search.pattern = nil
		return search
	}

	var err error
	hasSpecialChars := regexp.QuoteMeta(s) != s
	search.pattern, err = regexp.Compile(s)
	isValidRegexp := err == nil
	regexpMatchingRequired := hasSpecialChars && isValidRegexp
	search.isSubstringSearch = !regexpMatchingRequired

	search.hasUppercase = false
	for _, char := range s {
		if unicode.IsUpper(char) {
			search.hasUppercase = true
			break
		}
	}

	if search.isSubstringSearch {
		// Pattern still needed for GetMatchRanges()
		search.pattern, err = regexp.Compile(regexp.QuoteMeta(s))
		if err != nil {
			panic(err)
		}

		return search
	}

	// At this point we know it's a valid regexp, and that it does include
	// regexp specific characters. We also know the pattern has been
	// successfully compiled.

	return search
}

func (search *Search) Clear() {
	search.findMe = ""
	search.pattern = nil
}

func (search Search) Active() bool {
	return search.findMe != ""
}

func (search Search) Inactive() bool {
	return search.findMe == ""
}

func (search Search) Matches(line string) bool {
	if search.findMe == "" {
		return false
	}

	if search.isSubstringSearch && search.hasUppercase {
		// Case sensitive substring search
		return strings.Contains(line, search.findMe)
	}

	if search.isSubstringSearch && !search.hasUppercase {
		// Case insensitive substring search
		return strcase.Contains(line, search.findMe)
	}

	// Regexp search

	if !search.hasUppercase {
		// Regexp is already lowercase, do the same to the line to make the
		// search case insensitive
		line = strings.ToLower(line)
	}

	return search.pattern.MatchString(line)
}

// getMatchRanges locates one or more regexp matches in a string
func (search Search) GetMatchRanges(String string, idx linemetadata.Index) MatchRanges {
	if len(String) > 100000 {
		defer util.LogDuration("GetMatchRanges")()
	}

	if search.Inactive() {
		return MatchRanges{}
	}

	// Check the cache
	res, pres := search.Cache.Get(idx, search.findMe)
	if pres {
		return res
	}

	if !search.hasUppercase {
		// Case insensitive search, lowercase the string. The pattern is already
		// lowercase whenever hasUppercase is false.
		String = strings.ToLower(String)
	}

	res = MatchRanges{
		Matches: toRunePositions(search.pattern.FindAllStringIndex(String, -1), String),
	}
	search.Cache.Set(idx, search.findMe, res)

	return res
}

// Convert byte indices to rune indices
func toRunePositions(byteIndices [][]int, matchedString string) [][2]int {
	var returnMe [][2]int
	if len(byteIndices) == 0 {
		// Nothing to see here, move along
		return returnMe
	}

	runeIndex := 0
	byteIndicesToRuneIndices := make(map[int]int, 0)
	for byteIndex := range matchedString {
		byteIndicesToRuneIndices[byteIndex] = runeIndex

		runeIndex++
	}

	// If a match touches the end of the string, that will be encoded as one
	// byte past the end of the string. Therefore we must add a mapping for
	// first-index-after-the-end.
	byteIndicesToRuneIndices[len(matchedString)] = runeIndex

	for _, bytePair := range byteIndices {
		fromRuneIndex := byteIndicesToRuneIndices[bytePair[0]]
		toRuneIndex := byteIndicesToRuneIndices[bytePair[1]]
		returnMe = append(returnMe, [2]int{fromRuneIndex, toRuneIndex})
	}

	return returnMe
}
