package internal

import (
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/twin"
)

type PagerModeGotoLine struct {
	pager    *Pager
	inputBox InputBox
}

func NewPagerModeGotoLine(p *Pager) *PagerModeGotoLine {
	m := &PagerModeGotoLine{
		pager: p,
		inputBox: InputBox{
			accept:        INPUTBOX_ACCEPT_POSITIVE_NUMBERS,
			onTextChanged: nil,
		},
	}
	return m
}

func (m *PagerModeGotoLine) drawFooter(_ string, _ string, _ string) {
	m.inputBox.draw(m.pager.screen, "'ENTER' submits, 'ESC' cancels", "Go to line number: ")
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

func (m *PagerModeGotoLine) onKey(key twin.KeyCode) {
	if m.inputBox.handleKey(key) {
		return
	}

	switch key {
	case twin.KeyEnter:
		m.updateLineNumber(m.inputBox.text)
		m.pager.mode = PagerModeViewing{pager: m.pager}

	case twin.KeyEscape:
		m.pager.mode = PagerModeViewing{pager: m.pager}

	default:
		log.Tracef("Unhandled goto key event %v, treating as a viewing key event", key)
		m.pager.mode = PagerModeViewing{pager: m.pager}
		m.pager.mode.onKey(key)
	}
}

func (m *PagerModeGotoLine) onRune(char rune) {
	p := m.pager

	if char == 'q' {
		p.mode = PagerModeViewing{pager: p}
		return
	}

	if char == 'g' {
		p.scrollPosition = newScrollPosition("Pager scroll position")
		p.handleScrolledUp()
		p.mode = PagerModeViewing{pager: p}
		return
	}

	m.inputBox.handleRune(char)
}
