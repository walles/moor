package internal

import (
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
)

type PagerModeNotFound struct {
	pager *Pager
}

func (m PagerModeNotFound) drawFooter(_ string, _ string, _ string) {
	m.pager.setFooter("Not found: "+m.pager.search.String(), "", "", "")
}

func (m PagerModeNotFound) onKey(key twin.KeyCode) {
	action, found := m.pager.ModeBindings.NotFound.KeyCodeBindings[key]
	if !found {
		action, found = m.pager.ModeBindings.Viewing.KeyCodeBindings[key]
	}
	if found {
		m.executeAction(action)
		return
	}
	log.Debugf("Unhandled not-found key event %v", key)
}

func (m PagerModeNotFound) onRune(char rune) {
	action, found := m.pager.ModeBindings.NotFound.RuneBindings[char]
	if !found {
		action, found = m.pager.ModeBindings.Viewing.RuneBindings[char]
	}
	if found {
		m.executeAction(action)
		return
	}
	log.Debugf("Unhandled not-found rune '%s'/0x%08x", string(char), int32(char))
}

func (m PagerModeNotFound) executeAction(action Action) {
	switch action {
	case NextSearchHit:
		m.pager.mode = PagerModeViewing(m)
		m.pager.scrollToNextSearchHit(true)
	case PreviousSearchHit:
		m.pager.mode = PagerModeViewing(m)
		m.pager.scrollToPreviousSearchHit(true)
	default:
		// For all other actions, switch to viewing mode and execute there.
		viewing := PagerModeViewing(m)
		m.pager.mode = viewing
		viewing.executeAction(action)
	}
}
