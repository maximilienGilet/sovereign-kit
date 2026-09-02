package main

import (
	"bytes"
	"io"
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
func TestRunWithDelegatesSetupToApplication(t *testing.T) {
	input := strings.NewReader("setup input")
	var output bytes.Buffer
	var gotInput, gotOutput interface{}
	var gotPath, gotUser string
	app := application{
		setup: func(actualInput io.Reader, actualOutput io.Writer, path, user string) error {
			gotInput, gotOutput = actualInput, actualOutput
			gotPath, gotUser = path, user
			return nil
		},
	}
	if err := runWith([]string{"setup"}, input, &output, "/tmp/config.toml", "alice", app); err != nil {
		t.Fatal(err)
	}
	if gotInput != input || gotOutput != &output || gotPath != "/tmp/config.toml" || gotUser != "alice" {
		t.Fatalf("setup received input=%v output=%v path=%q user=%q", gotInput, gotOutput, gotPath, gotUser)
	}
}

func TestRunWithDelegatesStartToApplication(t *testing.T) {
	var output bytes.Buffer
	var gotOutput io.Writer
	var gotPath string
	app := application{
		start: func(actualOutput io.Writer, path string) error {
			gotOutput, gotPath = actualOutput, path
			return nil
		},
	}
	if err := runWith([]string{"start"}, strings.NewReader(""), &output, "/tmp/config.toml", "alice", app); err != nil {
		t.Fatal(err)
	}
	if gotOutput != &output || gotPath != "/tmp/config.toml" {
		t.Fatalf("start received output=%v path=%q", gotOutput, gotPath)
	}
}

func TestRunWithHelpConstructsNoApplicationResources(t *testing.T) {
	setupCalls, startCalls := 0, 0
	app := application{
		setup: func(io.Reader, io.Writer, string, string) error {
			setupCalls++
			return nil
		},
		start: func(io.Writer, string) error {
			startCalls++
			return nil
		},
	}
	var output bytes.Buffer
	if err := runWith(nil, strings.NewReader(""), &output, "/tmp/config.toml", "alice", app); err != nil {
		t.Fatal(err)
	}
	if err := runWith([]string{"help"}, strings.NewReader(""), &output, "/tmp/config.toml", "alice", app); err != nil {
		t.Fatal(err)
	}
	if setupCalls != 0 || startCalls != 0 {
		t.Fatalf("help invoked setup=%d start=%d", setupCalls, startCalls)
	}
}

func TestRunRejectsUnknownCommand(t *testing.T) {
	var output bytes.Buffer
	if err := run([]string{"unknown"}, &output); err == nil {
		t.Fatal("expected an error")
	}
}
