package twin

import (
	"io"
	"os"
	"sync/atomic"
	"time"
)

type interruptableReaderImpl struct {
	base *os.File

	interrupted atomic.Bool
}

const interruptableReaderPollInterval = 100 * time.Millisecond

func newInterruptableReader(base *os.File) (interruptableReader, error) {
	reader := &interruptableReaderImpl{
		base: base,
	}

	return reader, nil
}

func (r *interruptableReaderImpl) Interrupt() {
	r.interrupted.Store(true)

	log.Info("Interruptable reader interrupted")
}

func (r *interruptableReaderImpl) Read(p []byte) (n int, err error) {
	for {
		if r.interrupted.Load() {
			log.Info("Interruptable reader already interrupted, returning fabricated EOF")
			return 0, io.EOF
		}

		ready, waitErr := r.waitForReadReady()
		if waitErr != nil {
			return 0, waitErr
		}

		if !ready {
			continue
		}

		if r.interrupted.Load() {
			log.Info("Interruptable reader interrupted while waiting, returning fabricated EOF")
			return 0, io.EOF
		}

		n, err = r.base.Read(p)
		if r.interrupted.Load() {
			log.Info("Interruptable reader interrupted while reading, returning fabricated EOF")
			return 0, io.EOF
		}

		if err == io.EOF {
			log.Info("Interruptable reader base returned a genuine EOF")
		}

		return
	}
}
