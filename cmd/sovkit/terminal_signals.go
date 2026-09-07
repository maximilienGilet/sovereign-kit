package main

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/charmbracelet/x/term"
)

// The synchronous fallback must retain normal interrupt semantics even while
// ReadPassword is blocked. Restore borrowed terminal state before forwarding
// the signal; never leave a password-reading goroutine behind.
func guardLegacyTerminal(input io.Reader) func() {
	fd, ok := input.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(fd.Fd()) {
		return func() {}
	}
	state, err := term.GetState(fd.Fd())
	if err != nil {
		return func() {}
	}
	restore := func() { _ = term.Restore(fd.Fd(), state) }
	signals := make(chan os.Signal, 1)
	done := make(chan struct{})
	finished := make(chan struct{})
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		defer close(finished)
		restoreBeforeSignal(signals, done, restore, func(received os.Signal) {
			signal.Stop(signals)
			process, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = process.Signal(received)
			}
		})
	}()
	return func() { signal.Stop(signals); close(done); <-finished; restore() }
}

func restoreBeforeSignal(signals <-chan os.Signal, done <-chan struct{}, restore func(), forward func(os.Signal)) {
	select {
	case received := <-signals:
		restore()
		forward(received)
	case <-done:
	}
}
