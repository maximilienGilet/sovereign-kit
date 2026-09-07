package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/state"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

func runLogs(args []string, input io.Reader, output io.Writer, configPath string) error {
	set := flag.NewFlagSet("logs", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	follow := set.Bool("follow", false, "poll for new logs until interrupted")
	positionals, err := parseFlagsAroundPositional(set, args)
	if err != nil || len(positionals) > 1 {
		return usageErrorf("usage: sovkit logs [id] [--follow]")
	}
	id := ""
	if len(positionals) == 1 {
		id = positionals[0]
	}
	dir, _, deployment, err := resolveTarget(configPath, id)
	if err != nil {
		return err
	}
	token, err := state.Token(dir)
	if err != nil {
		return err
	}
	client := vast.NewClient(vastAPIBaseURL, token)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	text, err := client.GetDaemonLogs(ctx, deployment.Instance.ID)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, nonEmptyLogs(text)); err != nil {
		return err
	}
	if !*follow {
		return nil
	}
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	last := text
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-sigCtx.Done():
			return nil
		case <-ticker.C:
			fetchCtx, fetchCancel := context.WithTimeout(sigCtx, 30*time.Second)
			next, ferr := client.GetDaemonLogs(fetchCtx, deployment.Instance.ID)
			fetchCancel()
			if ferr != nil {
				fmt.Fprintf(output, "log fetch failed: %v\n", ferr)
				continue
			}
			if next == last {
				continue
			}
			last = next
			if _, err := fmt.Fprintln(output, nonEmptyLogs(next)); err != nil {
				return err
			}
		}
	}
}

func nonEmptyLogs(text string) string {
	if strings.TrimSpace(text) == "" {
		return "(no logs yet)"
	}
	return text
}
