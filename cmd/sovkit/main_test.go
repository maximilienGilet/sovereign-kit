package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

func TestInteractiveRoutesUseOneTerminalApplication(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		entry string
	}{{nil, "home"}, {[]string{"setup"}, "setup"}, {[]string{"start"}, "start"}, {[]string{"help"}, ""}, {[]string{"--help"}, ""}, {[]string{"-h"}, ""}, {[]string{"setup", "--help"}, ""}, {[]string{"start", "-h"}, ""}} {
		t.Run(strings.Join(tc.args, "/"), func(t *testing.T) {
			calls := 0
			input := strings.NewReader("")
			var output bytes.Buffer
			app := application{interactive: func(io.Reader, io.Writer) bool { return true }, terminal: func(_ context.Context, in io.Reader, out io.Writer, path, user, entry string) error {
				calls++
				if in != input || out != &output || path != "config" || user != "alice" || entry != tc.entry {
					t.Fatalf("wrong terminal arguments: %s %s %s", path, user, entry)
				}
				return nil
			}, setup: func(io.Reader, io.Writer, string, string) error { t.Fatal("legacy setup invoked"); return nil }, start: func(io.Writer, string) error { t.Fatal("legacy start invoked"); return nil }}
			if err := runWith(tc.args, input, &output, "config", "alice", app); err != nil {
				t.Fatal(err)
			}
			want := 1
			if tc.entry == "" {
				want = 0
			}
			if calls != want {
				t.Fatalf("terminal calls=%d want %d", calls, want)
			}
		})
	}
}

func TestBareNoninteractiveCommandShowsHelp(t *testing.T) {
	for _, env := range []struct{ key, value string }{{"TERM", "dumb"}, {"ACCESSIBLE", "1"}, {"TERM", "xterm-256color"}} {
		t.Run(env.key+env.value, func(t *testing.T) {
			t.Setenv(env.key, env.value)
			var output bytes.Buffer
			if err := runWith(nil, strings.NewReader(""), &output, "missing", "alice"); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "Usage:") {
				t.Fatalf("missing help: %s", output.String())
			}
		})
	}
}

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

func TestRunWithDashboardOpensClientLauncherWithoutStartingTunnel(t *testing.T) {
	input := strings.NewReader("dashboard input")
	var output bytes.Buffer
	startCalls, dashboardCalls := 0, 0
	app := application{
		start: func(io.Writer, string) error {
			startCalls++
			return nil
		},
		dashboard: func(actualInput io.Reader, actualOutput io.Writer) error {
			dashboardCalls++
			if actualInput != input || actualOutput != &output {
				t.Fatalf("dashboard received input=%v output=%v", actualInput, actualOutput)
			}
			return nil
		},
	}
	if err := runWith([]string{"dashboard"}, input, &output, "/tmp/config.toml", "alice", app); err != nil {
		t.Fatal(err)
	}
	if dashboardCalls != 1 || startCalls != 0 {
		t.Fatalf("dashboard calls=%d start calls=%d, want 1 and 0", dashboardCalls, startCalls)
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
