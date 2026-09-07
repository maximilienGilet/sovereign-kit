package main

import (
	"bytes"
	"os"
	"testing"
)

func TestSpinnerSilentOnPipePrintsFinalOnly(t *testing.T) {
	var output bytes.Buffer
	spin := newSpinner(&output, "Provisioning x")
	if spin.animate {
		t.Fatal("buffer must not animate")
	}
	spin.SetMessage("Waiting for instance ready")
	if output.String() != "" {
		t.Fatalf("silent until stop, got %q", output.String())
	}
	spin.Stop("Server launched")
	if output.String() != "Server launched\n" {
		t.Fatalf("final = %q", output.String())
	}
}

func TestSpinnerRendersFrameAndMessage(t *testing.T) {
	spin := &spinner{message: "Waiting", stop: make(chan struct{}), done: make(chan struct{})}
	if got := spin.render(); got != "\r⠋ Waiting" {
		t.Fatalf("frame = %q", got)
	}
	spin.frame = 3
	spin.SetMessage("Pinning")
	if got := spin.render(); got != "\r⠸ Pinning" {
		t.Fatalf("frame = %q", got)
	}
}

func TestIsTerminalFileDetectsCharacterDevices(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	if isTerminalFile(writer) {
		t.Fatal("pipe must not count as terminal")
	}
	null, err := os.Open("/dev/null")
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if !isTerminalFile(null) {
		t.Fatal("/dev/null must count as terminal")
	}
}
