package internal

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/walles/moor/v2/internal/linemetadata"
	"github.com/walles/moor/v2/internal/search"
	"github.com/walles/moor/v2/internal/textstyles"
	"github.com/walles/moor/v2/twin"
)

type PagerModeViewing struct {
	pager *Pager
}

func (m PagerModeViewing) drawFooter(filenameText string, statusText string, spinner string) {
	prefix := ""
	colonHelp := ""
	m.pager.readerLock.Lock()
	if len(m.pager.readers) > 1 {
		prefix = fmt.Sprintf("[%d/%d] ", m.pager.currentReader+1, len(m.pager.readers))
		colonKey := keyForAction(m.pager.ModeBindings.Viewing, ColonCommand)
		colonHelp = fmt.Sprintf("'%s' to switch, ", colonKey)
	}
	m.pager.readerLock.Unlock()

	vb := m.pager.ModeBindings.Viewing
	quitKey := keyForAction(vb, Quit)
	filterKey := keyForAction(vb, Filter)
	helpKey := keyForAction(vb, Help)
	searchKey := keyForAction(vb, SearchForward)
	nextKey := keyForAction(vb, NextSearchHit)
	prevKey := keyForAction(vb, PreviousSearchHit)

	searchHelp := fmt.Sprintf("'%s' to search", searchKey)
	if !m.pager.search.Inactive() {
		searchHelp = fmt.Sprintf("'%s'/'%s' to search next/previous", nextKey, prevKey)
	}
	helpText := fmt.Sprintf("Press '%s' to exit, ", quitKey) + colonHelp + searchHelp + fmt.Sprintf(", '%s' to filter, '%s' for help", filterKey, helpKey)

	if m.pager.isShowingHelp {
		helpText = fmt.Sprintf("Press '%s' to exit help, ", quitKey) + searchHelp
		prefix = ""
	}

	if m.pager.ShowStatusBar {
		if len(spinner) > 0 {
			spinner = "  " + spinner
		}
		m.pager.setFooter(prefix, filenameText, statusText+spinner, helpText)
	}
}

func (m PagerModeViewing) onKey(keyCode twin.KeyCode) {
	action, found := m.pager.ModeBindings.Viewing.KeyCodeBindings[keyCode]
	if found {
		m.executeAction(action)
		return
	}
	log.Debugf("Unhandled viewing key event %v", keyCode)
}

func (m PagerModeViewing) onRune(char rune) {
	action, found := m.pager.ModeBindings.Viewing.RuneBindings[char]
	if found {
		m.executeAction(action)
		return
	}
	log.Debugf("Unhandled viewing rune keypress '%s'/0x%08x", string(char), int32(char))
}

func (m PagerModeViewing) executeAction(action Action) {
	p := m.pager

	// Try to execute as common action first
	if p.executeCommonAction(action) {
		return
	}

	// Handle viewing-specific actions (mode switches)
	switch action {
	case SearchForward:
		p.mode = NewPagerModeSearch(p, SearchDirectionForward, p.scrollPosition)
		p.search.Clear()

		// Searchers want to scan the whole file, start reading as much as we can
		reallyHigh := linemetadata.IndexMax()
		p.setTargetLine(&reallyHigh)

	case SearchBackward:
		p.mode = NewPagerModeSearch(p, SearchDirectionBackward, p.scrollPosition)
		p.search.Clear()

		// Searchers want to scan the whole file, start reading as much as we can
		reallyHigh := linemetadata.IndexMax()
		p.setTargetLine(&reallyHigh)

	case Filter:
		if !p.isShowingHelp {
			// Filtering the help text is not supported. Feel free to work on
			// that if you feel that's time well spent.
			p.mode = NewPagerModeFilter(p)
			p.search.Clear()
			p.filter = search.Search{}
		}

	case GotoLine:
		p.mode = NewPagerModeGotoLine(p)
		p.setTargetLine(nil)

	case ColonCommand:
		if len(p.readers) > 1 {
			p.mode = &PagerModeColonCommand{pager: p}
			p.setTargetLine(nil)
		} else {
			p.mode = &PagerModeInfo{Pager: p, Text: "Pass more files on the command line to be able to switch between them."}
		}

	case NextSearchHit:
		p.scrollToNextSearchHit(false)

	case PreviousSearchHit:
		p.scrollToPreviousSearchHit(false)

	case Mark:
		p.mode = PagerModeMark{pager: p}
		p.setTargetLine(nil)

	case JumpToMark:
		p.mode = PagerModeJumpToMark{pager: p}
		p.setTargetLine(nil)

	default:
		log.Debugf("Unhandled viewing action: %d", action)
	}
}

func (p *Pager) cycleTabSize() {
	switch p.TabSize {
	case 8:
		p.TabSize = 4
	default:
		// We really want to toggle betwewen 4 and 8, but if we start out
		// somewhere else let's just go for 8. That's less' default tab size.
		p.TabSize = 8
	}
	textstyles.TabSize = p.TabSize

	p.mode = &PagerModeInfo{Pager: p, Text: fmt.Sprintf("Tab size set to %d", p.TabSize)}
}
