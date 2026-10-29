package textstyles

import (
	"strings"

	"github.com/walles/moor/v2/internal/linemetadata"
)

// A man page opens with a title line, with the same title and section at both
// ends: "LS(1)   General Commands Manual   LS(1)". groff may truncate long
// titles and run them into the middle text, but it truncates both ends the same
// way, so we compare the ends rather than splitting on whitespace.
//
// Across 8186 man pages formatted by groff, this matched all but the few that
// have no title line at all.
func IsManPageTitleLine(s string) bool {
	if len(s) > manPageFormattingScanBytes {
		// Title lines are terminal width, no need to strip megabyte lines
		return false
	}
	plain := strings.TrimSpace(StripFormatting(s, linemetadata.Index{}))

	// Find the end of "LS(1)", "IBUS EMOJI(7)", "gdbus()" or
	// "buildah-manifest-exists(1)()"
	titleEnd := strings.IndexByte(plain, '(')
	if titleEnd < 1 || strings.ContainsRune(plain[:titleEnd], ')') {
		return false
	}
	for titleEnd < len(plain) && plain[titleEnd] == '(' {
		sectionLength := strings.IndexByte(plain[titleEnd:], ')')
		if sectionLength < 0 || strings.ContainsAny(plain[titleEnd+1:titleEnd+sectionLength], " \t(") {
			return false
		}
		titleEnd += sectionLength + 1
	}
	title := plain[:titleEnd]

	// The middle text may be missing, glib's man pages have only spaces there
	return len(plain) > 2*len(title) && strings.HasSuffix(plain, title)
}
