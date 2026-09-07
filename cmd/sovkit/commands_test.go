package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpListsTheShimCommands(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"start", "tunnel", "doctor"} {
		if !strings.Contains(output.String(), command) {
			t.Fatalf("help does not describe %q: %s", command, output.String())
		}
	}
	for _, removed := range []string{"dashboard", "catalog", "setup", "resume"} {
		if strings.Contains(output.String(), removed) {
			t.Fatalf("help still describes removed %q: %s", removed, output.String())
		}
	}
}

func TestBareArgsShowUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}} {
		var output bytes.Buffer
		if err := runWith(args, strings.NewReader(""), &output, "missing"); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "Usage:") {
			t.Fatalf("missing usage: %s", output.String())
		}
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	for _, args := range [][]string{{"unknown"}, {"dashboard"}, {"catalog"}} {
		var output bytes.Buffer
		if err := run(args, &output); err == nil {
			t.Fatalf("expected an error for %q", args[0])
		}
	}
}
