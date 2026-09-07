package main

import (
	"context"
	"io"
	"strconv"
	"strings"
	"testing"
)

func TestResumeDispatchNeverCreates(t *testing.T) {
	for _, interactive := range []bool{false, true} {
		for _, id := range []int{0, 49834278} {
			args := []string{"resume"}
			entry := "resume"
			if id > 0 {
				args = append(args, strconv.Itoa(id))
				entry += ":" + strconv.Itoa(id)
			}
			calls := 0
			app := application{
				interactive: func(io.Reader, io.Writer) bool { return interactive },
				setup:       func(io.Reader, io.Writer, string, string) error { t.Fatal("unexpected setup"); return nil },
				resume: func(_ io.Reader, _ io.Writer, _ string, _ string, got int) error {
					calls++
					if interactive || got != id {
						t.Fatalf("wrong resume dispatch %d", got)
					}
					return nil
				},
				terminal: func(_ context.Context, _ io.Reader, _ io.Writer, _ string, _ string, got string) error {
					calls++
					if !interactive || got != entry {
						t.Fatalf("wrong terminal entry %s", got)
					}
					return nil
				},
			}
			if err := runWith(args, strings.NewReader(""), io.Discard, "config", "alice", app); err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("dispatch count %d", calls)
			}
		}
	}
}

func TestResumeRejectsInvalidIDsWithoutDispatch(t *testing.T) {
	for _, args := range [][]string{{"resume", "0"}, {"resume", "-1"}, {"resume", "abc"}, {"resume", "1", "2"}} {
		app := application{resume: func(io.Reader, io.Writer, string, string, int) error {
			t.Fatal("invalid resume dispatched")
			return nil
		}}
		if err := runWith(args, strings.NewReader(""), io.Discard, "config", "alice", app); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
