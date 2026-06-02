package internal

import (
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
)

type PagerModeColonCommand struct {
	pager *Pager
}

func (m *PagerModeColonCommand) drawFooter(_ string, _ string, _ []twin.StyledRune) {
	p := m.pager
	_, screenHeight := p.ScreenSize()
	height := int(screenHeight)

	pos := 0
	for _, token := range "Go to [n]ext, [p]revious or first [x] file: " {
		pos += p.screen.SetCell(pos, height-1, twin.NewStyledRune(token, twin.StyleDefault))
	}

	// Add a cursor
	p.screen.SetCell(pos, height-1, twin.NewStyledRune(' ', twin.StyleDefault.WithAttr(twin.AttrReverse)))
}

func (m *PagerModeColonCommand) onKey(key twin.KeyCode) {
	p := m.pager

	switch key {
	case twin.KeyEscape:
		p.mode = PagerModeViewing{pager: p}

	default:
		log.Tracef("Unhandled colon command event %v, treating as a viewing key event", key)
		p.mode = PagerModeViewing{pager: p}
		p.mode.onKey(key)
	}
}

func (m *PagerModeColonCommand) onRune(char rune) {
	p := m.pager

	if char == 'q' {
		// Back to viewing mode, just like ESC
		p.mode = PagerModeViewing{pager: p}
	}

	if char == 'p' {
		p.mode = PagerModeViewing{pager: p}
		p.previousFile()
		return
	}

	if char == 'n' {
		p.mode = PagerModeViewing{pager: p}
		p.nextFile()
		return
	}

	if char == 'x' {
		p.mode = PagerModeViewing{pager: p}
		p.firstFile()
		return
	}

	log.Debugf("Unhandled colon command rune %q, ignoring it", char)
}
