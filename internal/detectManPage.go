package internal

import (
	"github.com/walles/moor/v2/internal/linemetadata"
)

// How many lines from the top we look for man page formatting. A man page opens
// with a title line and a blank line, and its first formatted line can be a few
// paragraphs down; across 14801 man pages from /usr/share/man and Homebrew the
// deepest was line 13, and looking further found no additional man pages.
const manPageDetectionLines = 14

func (p *Pager) haveLoadedManPage() bool {
	reader := p.Reader()
	for _, line := range reader.GetLines(linemetadata.Index{}, manPageDetectionLines).Lines {
		if line.Line.HasManPageFormatting() || line.Line.IsManPageTitleLine() {
			return true
		}
	}
	return false
}
