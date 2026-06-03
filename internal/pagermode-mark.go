package internal

import (
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
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
		pos += p.screen.SetCell(pos, height-1, twin.NewStyledRune(token, twin.StyleDefault))
	}

	// Add a cursor
	p.screen.SetCell(pos, height-1, twin.NewStyledRune(' ', twin.StyleDefault.WithAttr(twin.AttrReverse)))
}

func (m PagerModeMark) executeAction(action Action) {
	p := m.pager

	switch action {
	case Cancel:
		p.mode = PagerModeViewing{pager: p}
		return
	}

	// Try common actions (including NoAction)
	if p.executeCommonAction(action) {
		return
	}

	// Action not handled
	log.Debugf("Unhandled mark action: %v", action)
}

func (m PagerModeMark) onKey(key twin.KeyCode) {
	p := m.pager

	action, found := p.ModeBindings.Mark.KeyCodeBindings[key]
	if found {
		m.executeAction(action)
		return
	}

	// No action bound, fall through to viewing mode
	log.Tracef("Unhandled mark key event %v, treating as a viewing key event", key)
	p.mode = PagerModeViewing{pager: p}
	p.mode.onKey(key)
}

func (m PagerModeMark) onRune(char rune) {
	log.Debugf("Setting mark '%s' at %v", string(char), m.pager.scrollPosition)
	m.pager.bookmarks[char] = m.pager.scrollPosition
	m.pager.mode = PagerModeViewing(m)
}
