package internal

import (
	"fmt"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/twin"
)

type PagerModeGotoLine struct {
	pager    *Pager
	inputBox InputBox
}

func NewPagerModeGotoLine(p *Pager) *PagerModeGotoLine {
	m := &PagerModeGotoLine{
		pager:    p,
		inputBox: *NewInputBox(INPUTBOX_ACCEPT_POSITIVE_NUMBERS, p.ModeBindings.Input),
	}
	return m
}

func (m *PagerModeGotoLine) drawFooter(_ string, _ string, _ string) {
	acceptKey := keyForAction(m.pager.ModeBindings.GotoLine, Accept)
	cancelKey := keyForAction(m.pager.ModeBindings.GotoLine, Cancel)
	hint := fmt.Sprintf("'%s' submits, '%s' cancels", acceptKey, cancelKey)
	m.inputBox.draw(m.pager.screen, hint, "Go to line number: ")
}

func (m *PagerModeGotoLine) updateLineNumber(text string) {
	newLineNumber, err := strconv.Atoi(text)
	if err != nil {
		log.Debugf("Got non-number goto text '%s'", text)
		return
	}
	if newLineNumber < 1 {
		log.Debugf("Got non-positive goto line number: %d", newLineNumber)
		return
	}
	targetIndex := linemetadata.IndexFromOneBased(newLineNumber)
	m.pager.scrollPosition = NewScrollPositionFromIndex(
		targetIndex,
		"onGotoLineKey",
	)
	m.pager.setTargetLine(&targetIndex)
}

func (m *PagerModeGotoLine) executeAction(action Action) {
	switch action {
	case Accept:
		m.updateLineNumber(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		return

	case Cancel:
		m.pager.mode = PagerModeViewing{pager: m.pager}
		return

	case GotoTop:
		m.pager.scrollPosition = newScrollPosition("Pager scroll position")
		m.pager.handleScrolledUp()
		m.pager.mode = PagerModeViewing{pager: m.pager}
		return

	}

	// Try common actions
	if m.pager.executeCommonAction(action) {
		return
	}

	// Action not handled
	log.Debugf("Unhandled goto-line action: %v", action)
}

func (m *PagerModeGotoLine) onKey(key twin.KeyCode) {
	p := m.pager

	// Check keybindings first (priority over InputBox)
	if action, found := p.ModeBindings.GotoLine.KeyCodeBindings[key]; found {
		m.executeAction(action)
		return
	}

	// Fall back to InputBox handling
	if m.inputBox.handleKey(key) {
		return
	}

	// No action bound, fall through to viewing mode
	log.Tracef("Unhandled goto-line key event %v, treating as a viewing key event", key)
	p.mode = PagerModeViewing{pager: p}
	p.mode.onKey(key)
}

func (m *PagerModeGotoLine) onRune(char rune) {
	// Look up action from keybindings
	action, found := m.pager.ModeBindings.GotoLine.RuneBindings[char]
	if found {
		m.executeAction(action)
		return
	}

	// Unbound rune - fallthrough to number input
	m.inputBox.handleRune(char)
}
