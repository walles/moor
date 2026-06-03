package internal

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/twin"
)

type PagerModeColonCommand struct {
	pager *Pager
}

func (m *PagerModeColonCommand) drawFooter(_ string, _ string, _ string) {
	p := m.pager
	_, screenHeight := p.ScreenSize()
	height := int(screenHeight)

	nextKey := keyForAction(p.ModeBindings.ColonCommand, NextFile)
	prevKey := keyForAction(p.ModeBindings.ColonCommand, PreviousFile)
	firstKey := keyForAction(p.ModeBindings.ColonCommand, FirstFile)
	prompt := fmt.Sprintf("Go to next [%s], previous [%s] or first [%s] file: ", nextKey, prevKey, firstKey)

	pos := 0
	for _, token := range prompt {
		pos += p.screen.SetCell(pos, height-1, twin.NewStyledRune(token, twin.StyleDefault))
	}

	// Add a cursor
	p.screen.SetCell(pos, height-1, twin.NewStyledRune(' ', twin.StyleDefault.WithAttr(twin.AttrReverse)))
}

func (m *PagerModeColonCommand) executeAction(action Action) {
	p := m.pager

	switch action {
	case Cancel:
		p.mode = PagerModeViewing{pager: p}
		return

	case PreviousFile:
		p.mode = PagerModeViewing{pager: p}
		p.previousFile()
		return

	case NextFile:
		p.mode = PagerModeViewing{pager: p}
		p.nextFile()
		return

	case FirstFile:
		p.mode = PagerModeViewing{pager: p}
		p.firstFile()
		return
	}

	// Try common actions
	if p.executeCommonAction(action) {
		return
	}

	// Action not handled
	log.Debugf("Unhandled colon command action: %v", action)
}

func (m *PagerModeColonCommand) onKey(key twin.KeyCode) {
	p := m.pager

	// Look up action from keybindings
	action, found := p.ModeBindings.ColonCommand.KeyCodeBindings[key]
	if found {
		m.executeAction(action)
		return
	}

	// No action bound, fall through to viewing mode
	log.Tracef("Unhandled colon command event %v, treating as a viewing key event", key)
	p.mode = PagerModeViewing{pager: p}
	p.mode.onKey(key)
}

func (m *PagerModeColonCommand) onRune(char rune) {
	p := m.pager

	// Look up action from keybindings
	action, found := p.ModeBindings.ColonCommand.RuneBindings[char]
	if found {
		m.executeAction(action)
		return
	}

	// No action bound, ignore it
	log.Debugf("Unhandled colon command rune %q, ignoring it", char)
}
