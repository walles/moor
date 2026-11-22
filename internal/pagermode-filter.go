package internal

import (
	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/twin"
)

type PagerModeFilter struct {
	pager                 *Pager
	initialScrollPosition scrollPosition // Pager position before filtering started
	inputBox              *InputBox
	history               *HistoryNavigator
}

func NewPagerModeFilter(p *Pager, initialScrollPosition scrollPosition) *PagerModeFilter {
	m := &PagerModeFilter{
		pager:                 p,
		initialScrollPosition: initialScrollPosition,
		history:               NewHistoryNavigator(p.searchHistory),
	}
	m.inputBox = &InputBox{
		accept: INPUTBOX_ACCEPT_ALL,
		onTextChanged: func(text string) {
			m.updateFilterPattern(text)
		},
	}
	return m
}

func (m PagerModeFilter) drawFooter(_ string, _ string, _ string) {
	m.inputBox.draw(m.pager.screen, "Type to filter, 'ENTER' submits, 'ESC' cancels", "Filter: ")
}

func (m *PagerModeFilter) updateFilterPattern(text string) {
	m.pager.filter.For(text)
	m.pager.search.For(text)
}

func (m *PagerModeFilter) onKey(key twin.KeyCode) {
	if m.inputBox.handleKey(key) {
		m.history.TextEdited(m.inputBox.text)
		return
	}

	switch key {
	case twin.KeyEnter:
		m.history.Commit(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}

	case twin.KeyEscape:
		m.history.Commit(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.filter = search.Search{}
		m.pager.search.Clear()
		m.pager.scrollPosition = m.initialScrollPosition

	case twin.KeyUp:
		if text, ok := m.history.Move(-1); ok {
			m.inputBox.setText(text)
		}

	case twin.KeyDown:
		if text, ok := m.history.Move(1); ok {
			m.inputBox.setText(text)
		}

	case twin.KeyPgUp, twin.KeyPgDown:
		m.history.Commit(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.mode.onKey(key)

	default:
		log.Debugf("Unhandled filter key event %v", key)
	}
}

func (m *PagerModeFilter) onRune(char rune) {
	m.inputBox.handleRune(char)
	m.history.TextEdited(m.inputBox.text)
}
