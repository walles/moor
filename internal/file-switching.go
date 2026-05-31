package internal

import (
	log "github.com/sirupsen/logrus"
)

func (p *Pager) previousFile() {
	p.readerLock.Lock()
	defer p.readerLock.Unlock()

	newIndex := p.currentReader - 1
	if newIndex < 0 {
		newIndex = 0
	}
	p.switchToFile(newIndex)
	log.Tracef("Switched to previous file, index %d", p.currentReader)

	select {
	case p.readerSwitched <- struct{}{}:
	default:
	}
}

func (p *Pager) nextFile() {
	p.readerLock.Lock()
	defer p.readerLock.Unlock()

	newIndex := p.currentReader + 1
	if newIndex >= len(p.readers) {
		newIndex = len(p.readers) - 1
	}
	p.switchToFile(newIndex)
	log.Tracef("Switched to next file, index %d", p.currentReader)

	select {
	case p.readerSwitched <- struct{}{}:
	default:
	}
}

func (p *Pager) firstFile() {
	p.readerLock.Lock()
	defer p.readerLock.Unlock()

	p.switchToFile(0)
	log.Tracef("Switched to first file, index %d", p.currentReader)

	select {
	case p.readerSwitched <- struct{}{}:
	default:
	}
}

func (p *Pager) switchToFile(newIndex int) {
	if newIndex == p.currentReader {
		return
	}

	p.currentReader = newIndex
	p.scrollPosition = newScrollPosition("Pager file switch")
}
