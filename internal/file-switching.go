package internal

import (
	log "github.com/sirupsen/logrus"
)

func (p *Pager) previousFile() {
	p.switchToFile(p.currentReader - 1)
}

func (p *Pager) nextFile() {
	p.switchToFile(p.currentReader + 1)
}

func (p *Pager) firstFile() {
	p.switchToFile(0)
}

// Save the state of the current reader and restore the state of the new one.
//
// Must be called from the main loop goroutine, which is also the only one
// changing currentReader. That's why we can read it without locking.
func (p *Pager) switchToFile(newIndex int) {
	newIndex = max(0, min(newIndex, len(p.views)-1))
	if newIndex == p.currentReader {
		return
	}

	// Save where we are. If we're on our way somewhere, like when following,
	// remember that rather than where we happen to be right now.
	leaving := &p.views[p.currentReader]
	leaving.targetLine = p.TargetLine
	if leaving.targetLine == nil {
		leaving.targetLine = p.lineIndex()
	}
	leaving.leftColumnZeroBased = p.leftColumnZeroBased
	leaving.filter = p.filter

	p.readerLock.Lock()
	p.currentReader = newIndex
	p.filteringReader.SetBackingReader(p.views[newIndex].reader)
	p.readerLock.Unlock()

	entering := p.views[newIndex]
	p.filter = entering.filter
	p.leftColumnZeroBased = entering.leftColumnZeroBased
	p.scrollPosition = newScrollPosition("Pager file switch")
	p.setTargetLine(entering.targetLine)

	// Go to the target line now if we can, rather than showing the top of the
	// file until more lines arrive
	p.handleMoreLinesAvailable()

	log.Tracef("Switched to file index %d", p.currentReader)

	select {
	case p.readerSwitched <- struct{}{}:
	default:
	}
}
