package internal

import (
	"github.com/walles/moor/v2/twin"
)

type PagerModeNotFound struct {
	pager *Pager
}

func (m PagerModeNotFound) drawFooter(_ string, _ string, _ []twin.StyledRune) {
	m.pager.setFooter("Not found: "+m.pager.search.String(), "", "", nil, "")
}

func (m PagerModeNotFound) onKey(key twin.KeyCode) {
	m.pager.mode = PagerModeViewing(m)
	m.pager.mode.onKey(key)
}

func (m PagerModeNotFound) onRune(char rune) {
	switch char {

	// Should match the pagermode-viewing.go next-search-hit bindings
	case 'n':
		m.pager.scrollToNextSearchHit()

	// Should match the pagermode-viewing.go previous-search-hit bindings
	case 'p', 'N':
		m.pager.scrollToPreviousSearchHit()

	default:
		m.pager.mode = PagerModeViewing(m)
		m.pager.mode.onRune(char)
	}
}
