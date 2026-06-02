// Show some info to the user. Fall back to viewing on any input.

package internal

import (
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
)

type PagerModeInfo struct {
	Pager  *Pager
	Text   string
	logged bool
}

func (m *PagerModeInfo) drawFooter(_ string, _ string, _ []twin.StyledRune) {
	if !m.logged {
		log.Infof("Displaying info message to user: %q", m.Text)
		m.logged = true
	}

	m.Pager.setFooter(m.Text, "", "", nil, "")
}

func (m *PagerModeInfo) onKey(key twin.KeyCode) {
	m.Pager.mode = PagerModeViewing{m.Pager}
	m.Pager.mode.onKey(key)
}

func (m *PagerModeInfo) onRune(char rune) {
	m.Pager.mode = PagerModeViewing{m.Pager}
	m.Pager.mode.onRune(char)
}
