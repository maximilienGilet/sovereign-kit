package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunExplainsTheCatalogCommand(t *testing.T) {
	var output bytes.Buffer
	err := run([]string{"help"}, &output)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "catalog") {
		t.Fatalf("help does not describe catalog: %s", output.String())
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"unknown"}, &output); err == nil {
		t.Fatal("expected an error")
	}
}
