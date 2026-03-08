//go:build !windows

package twin

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

type interruptableReaderImpl struct {
	base *os.File

	shutdownPipeReader *os.File
	shutdownPipeWriter *os.File

	wakeupPipeReader *os.File
	wakeupPipeWriter *os.File

	interrupted atomic.Bool
	wakeupSent  atomic.Bool
}

func (r *interruptableReaderImpl) Read(p []byte) (n int, err error) {
	for {
		if r.interrupted.Load() {
			log.Info("Interruptable reader already interrupted, returning fabricated EOF")
			return 0, io.EOF
		}

		n, err = r.read(p)

		if err == syscall.EINTR {
			// Not really a problem, we can get this on window resizes for
			// example, just try again.
			continue
		}

		return
	}
}

func (r *interruptableReaderImpl) read(p []byte) (n int, err error) {
	// "This argument should be set to the highest-numbered file descriptor in
	// any of the three sets, plus 1. The indicated file descriptors in each set
	// are checked, up to this limit"
	//
	// Ref: https://man7.org/linux/man-pages/man2/select.2.html
	nfds := max(r.wakeupPipeReader.Fd(), r.shutdownPipeReader.Fd(), r.base.Fd())

	readFds := unix.FdSet{}
	readFds.Set(int(r.shutdownPipeReader.Fd()))
	readFds.Set(int(r.wakeupPipeReader.Fd()))
	readFds.Set(int(r.base.Fd()))

	_, err = unix.Select(int(nfds)+1, &readFds, nil, nil, nil)
	if err != nil {
		// Select failed
		return
	}

	if readFds.IsSet(int(r.shutdownPipeReader.Fd())) {
		// Shutdown requested
		closeErr := r.shutdownPipeReader.Close()
		if closeErr != nil {
			// This should never happen, but if it does we should log it
			log.Error(fmt.Sprint("Failed to close shutdown pipe reader: ", closeErr))
		} else {
			log.Info("Interruptable reader shutdown pipe reader closed, returning fabricated EOF")
		}

		err = io.EOF

		return
	}

	if readFds.IsSet(int(r.wakeupPipeReader.Fd())) {
		buffer := make([]byte, 1)
		_, readErr := r.wakeupPipeReader.Read(buffer)
		if readErr != nil {
			log.Info(fmt.Sprint("Failed to consume wakeup byte: ", readErr))
		}

		r.wakeupSent.Store(false)
		return 0, nil
	}

	if readFds.IsSet(int(r.base.Fd())) {
		// Base has stuff
		n, err = r.base.Read(p)

		if err == io.EOF {
			log.Info("Interruptable reader base returned a genuine EOF")
		}

		return
	}

	// Neither base nor shutdown pipe was ready, this should never happen
	return
}

func (r *interruptableReaderImpl) Interrupt() {
	r.interrupted.Store(true)

	err := r.shutdownPipeWriter.Close()
	if err != nil {
		// This should never happen, but if it does we should log it
		log.Info(fmt.Sprint("Failed to close shutdown pipe writer: ", err))
		return
	}

	log.Info("Interruptable reader interrupted")
}

func (r *interruptableReaderImpl) Wakeup() error {
	if r.interrupted.Load() {
		return nil
	}

	if !r.wakeupSent.CompareAndSwap(false, true) {
		// Wakeup already pending
		return nil
	}

	_, err := r.wakeupPipeWriter.Write([]byte{1})
	if err != nil {
		r.wakeupSent.Store(false)
		return fmt.Errorf("failed to send wakeup byte: %w", err)
	}

	return nil
}

func newInterruptableReader(base *os.File) (interruptableReader, error) {
	reader := interruptableReaderImpl{
		base: base,
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	reader.shutdownPipeReader = pr
	reader.shutdownPipeWriter = pw

	wakeupReader, wakeupWriter, err := os.Pipe()
	if err != nil {
		closeErr := reader.shutdownPipeReader.Close()
		if closeErr != nil {
			log.Error(fmt.Sprint("Failed to close shutdown pipe reader: ", closeErr))
		}

		closeErr = reader.shutdownPipeWriter.Close()
		if closeErr != nil {
			log.Error(fmt.Sprint("Failed to close shutdown pipe writer: ", closeErr))
		}

		return nil, err
	}

	reader.wakeupPipeReader = wakeupReader
	reader.wakeupPipeWriter = wakeupWriter

	return &reader, nil
}

// Subscribe to SIGWINCH signals. Compared to polling, this will reduce power
// usage in the absence of window resizes.
func (screen *UnixScreen) setupSigwinchNotification() {
	screen.sigwinch = make(chan int, 1)
	screen.sigwinch <- 0 // Trigger initial screen size query

	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	go func() {
		defer func() {
			panicHandler("setupSigwinchNotification()/SIGWINCH", recover(), debug.Stack())
		}()

		for {
			// Await window resize signal
			<-sigwinch

			screen.onWindowResized()
		}
	}()
}

func (screen *UnixScreen) setupTtyInTtyOut() error {
	// Dup stdout so we can close stdin in Close() without closing stdout.
	// Before this dupping, we crashed on using --quit-if-one-screen.
	//
	// Ref:https://github.com/walles/moor/issues/214
	stdoutDupFd, err := syscall.Dup(int(os.Stdout.Fd()))
	if err != nil {
		return err
	}
	stdoutDup := os.NewFile(uintptr(stdoutDupFd), "moor-stdout-dup")

	// os.Stdout is a stream that goes to our terminal window.
	//
	// So if we read from there, we'll get input from the terminal window.
	//
	// If we just read from os.Stdin that would fail when getting data piped
	// into ourselves from some other command.
	//
	// Tested on macOS and Linux, works like a charm!
	screen.ttyIn = stdoutDup // <- YES, WE SHOULD ASSIGN STDOUT TO TTYIN

	// Set input stream to raw mode
	screen.oldTerminalState, err = term.MakeRaw(int(screen.ttyIn.Fd()))
	if err != nil {
		return err
	}

	screen.ttyOut = os.Stdout

	ttyInTerminalState, err := term.GetState(int(screen.ttyIn.Fd()))
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("ttyin terminal state: %+v", ttyInTerminalState))

	ttyOutTerminalState, err := term.GetState(int(screen.ttyOut.Fd()))
	if err != nil {
		return err
	}
	log.Info(fmt.Sprintf("ttyout terminal state: %+v", ttyOutTerminalState))

	return nil
}

func (screen *UnixScreen) restoreTtyInTtyOut() error {
	return term.Restore(int(screen.ttyIn.Fd()), screen.oldTerminalState)
}
