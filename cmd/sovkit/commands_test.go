package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelpListsTheOperationalRouteCommands(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"help"}, &output); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"setup", "dashboard", "doctor", "tunnel"} {
		if !strings.Contains(output.String(), command) {
			t.Fatalf("help does not describe %q: %s", command, output.String())
		}
	}
}
