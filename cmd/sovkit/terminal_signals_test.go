package main

import (
	"os"
	"reflect"
	"testing"
)

func TestLegacySignalRestoresTerminalBeforeForwardingInterrupt(t *testing.T) {
	signals := make(chan os.Signal, 1)
	signals <- os.Interrupt
	var events []string
	restoreBeforeSignal(signals, make(chan struct{}), func() { events = append(events, "restore") }, func(signal os.Signal) {
		if signal != os.Interrupt {
			t.Fatalf("signal=%v", signal)
		}
		events = append(events, "forward")
	})
	if !reflect.DeepEqual(events, []string{"restore", "forward"}) {
		t.Fatalf("events=%v", events)
	}
}

func TestLegacySignalGuardStopsWithoutForwardingOnCompletion(t *testing.T) {
	done := make(chan struct{})
	close(done)
	restoreBeforeSignal(make(chan os.Signal), done, func() { t.Fatal("unexpected signal restoration") }, func(os.Signal) { t.Fatal("unexpected signal forwarding") })
}
