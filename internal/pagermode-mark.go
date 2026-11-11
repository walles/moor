package internal

import (
	"github.com/walles/twin"

	"github.com/walles/moor/v2/internal/linemetadata"
)

type PagerModeMark struct {
	pager *Pager
}

func (m PagerModeMark) drawFooter(_ string, _ string, _ string) {
	p := m.pager

	_, screenHeight := p.ScreenSize()
	height := int(screenHeight)

	pos := 0
	for _, token := range "Press any key to label your mark: " {
		pos += p.screen.SetCell(pos, height-1, twin.StyledRune{Rune: token, Style: twin.StyleDefault})
	}

	// Add a cursor
	p.screen.SetCell(pos, height-1, twin.StyledRune{Rune: ' ', Style: twin.StyleDefault.WithAttr(twin.AttrReverse)})
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
	p := m.pager

	lineNumber := linemetadata.NumberFromZeroBased(0)
	if index := p.lineIndex(); index != nil {
		if line := p.Reader().GetLine(*index); line != nil {
			lineNumber = line.Number
		}
	}

	p.bookmarks[char] = bookmark{view: p.currentReader, line: lineNumber}
	p.mode = PagerModeViewing(m)
}
