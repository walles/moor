package internal

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/twin"
)

type PagerModeFilter struct {
	pager    *Pager
	inputBox *InputBox
}

func NewPagerModeFilter(p *Pager) *PagerModeFilter {
	m := &PagerModeFilter{
		pager: p,
	}
	m.inputBox = NewInputBox(INPUTBOX_ACCEPT_ALL, p.ModeBindings.Input)
	m.inputBox.onTextChanged = func(text string) {
		m.updateFilterPattern(text)
	}
	return m
}

func (m PagerModeFilter) drawFooter(_ string, _ string, _ string) {
	acceptKey := keyForAction(m.pager.ModeBindings.Filter, Accept)
	cancelKey := keyForAction(m.pager.ModeBindings.Filter, Cancel)
	hint := fmt.Sprintf("Type to filter, '%s' submits, '%s' cancels", acceptKey, cancelKey)
	m.inputBox.draw(m.pager.screen, hint, "Filter: ")
}

func (m *PagerModeFilter) updateFilterPattern(text string) {
	m.pager.filter.For(text)
	m.pager.search.For(text)
}

func (m *PagerModeFilter) executeAction(action Action) {
	switch action {
	case Accept:
		m.pager.mode = PagerModeViewing{pager: m.pager}
		return

	case Cancel:
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.filter = search.Search{}
		m.pager.search.Clear()
		return
	}

	// Try common actions
	if m.pager.executeCommonAction(action) {
		return
	}

	// Action not handled
	log.Debugf("Unhandled filter action: %v", action)
}

func (m *PagerModeFilter) onKey(key twin.KeyCode) {
	// Check keybindings first (priority over InputBox)
	if action, found := m.pager.ModeBindings.Filter.KeyCodeBindings[key]; found {
		m.executeAction(action)
		return
	}

	// Fall back to InputBox handling
	if m.inputBox.handleKey(key) {
		return
	}

	log.Debugf("Unhandled filter key event %v", key)
}

func (m *PagerModeFilter) onRune(char rune) {
	// Look up action from keybindings
	action, found := m.pager.ModeBindings.Filter.RuneBindings[char]
	if found {
		m.executeAction(action)
		return
	}

	// Unbound rune - fallthrough to text input
	m.inputBox.handleRune(char)
}
