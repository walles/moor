package internal

import (
	"github.com/walles/moor/v2/twin"
)

type PagerModeMark struct {
	pager *Pager
}

func (m PagerModeMark) drawFooter(_ string, _ string, _ []twin.StyledRune) {
	p := m.pager

	_, screenHeight := p.ScreenSize()
	height := int(screenHeight)

	pos := 0
	for _, token := range "Press any key to label your mark: " {
		pos += p.screen.SetCell(pos, height-1, twin.NewStyledRune(token, twin.StyleDefault))
	}

	// Add a cursor
	p.screen.SetCell(pos, height-1, twin.NewStyledRune(' ', twin.StyleDefault.WithAttr(twin.AttrReverse)))
}

func (m PagerModeMark) onKey(key twin.KeyCode) {
	p := m.pager

	switch key {
	case twin.KeyEnter, twin.KeyEscape:
		// Never mind I
		p.mode = PagerModeViewing{pager: p}

	default:
		// Never mind II
		p.mode = PagerModeViewing{pager: p}
		p.mode.onKey(key)
	}
}

func (m PagerModeMark) onRune(char rune) {
	m.pager.bookmarks[char] = m.pager.scrollPosition
	m.pager.mode = PagerModeViewing(m)
}
