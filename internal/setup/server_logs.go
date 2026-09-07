package setup

import (
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type ServerLogs struct {
	Text      string
	CheckedAt time.Time
	Download  *DownloadProgress
}
type ServerLogObserver interface{ ServerLogSnapshot(ServerLogs) }

func (launcher StrictSSHLauncher) captureServerLogs(ctx context.Context, ssh config.SSH, engine string) {
	if launcher.OnLogs == nil || ctx.Err() != nil {
		return
	}
	readCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := launcher.Runner.Output(readCtx, strictSSHCommand(ssh, "tail -c 16384 "+shellQuote("/workspace/sovkit-"+engine+".log")))
	text := cleanServerLogs(string(output), launcher.LogSecrets)
	if err != nil && text == "" {
		text = "Server logs could not be read (missing file or SSH unavailable)."
	}
	if err == nil && text == "" {
		text = "Server log file is empty."
	}
	launcher.OnLogs(ServerLogs{Text: text, CheckedAt: time.Now(), Download: ParseDownloadProgress(text)})
}

var logCredentials = regexp.MustCompile(`(?i)(bearer\s+|(?:api[_-]?key|access[_-]?token|token|password|secret)\s*[=:]\s*)[^\s,;]+`)
var hfCredential = regexp.MustCompile(`\bhf_[A-Za-z0-9]{8,}\b`)

func cleanServerLogs(raw string, secrets []string) string {
	for _, secret := range secrets {
		if strings.TrimSpace(secret) != "" {
			raw = strings.ReplaceAll(raw, strings.TrimSpace(secret), "[redacted]")
		}
	}
	raw = ansi.Strip(raw)
	raw = logCredentials.ReplaceAllString(raw, "${1}[redacted]")
	raw = hfCredential.ReplaceAllString(raw, "[redacted]")
	raw = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			return r
		}
		return -1
	}, raw)
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], 500, "…")
	}
	return strings.Join(lines, "\n")
}

// One cancellable polling loop reads a bounded snapshot and checks readiness.
// No tail process or background SSH session survives cancellation.
func (launcher StrictSSHLauncher) waitServerReady(ctx context.Context, ssh config.SSH, engine string) error {
	return launcher.waitServerReadyWithAdvice(ctx, ssh, engine, "")
}

func (launcher StrictSSHLauncher) waitServerReadyWithAdvice(ctx context.Context, ssh config.SSH, engine, oomAdvice string) error {
	clock := launcher.Clock
	if clock == nil {
		clock = RealClock{}
	}
	deadline := clock.Now().Add(30 * time.Minute)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	stem := "/workspace/sovkit-" + engine
	command := `if curl --fail --silent --max-time 2 http://127.0.0.1:30000/health >/dev/null 2>&1; then printf 'ready\n'; elif [ -s ` + shellQuote(stem+".pid") + ` ] && ! kill -0 "$(cat ` + shellQuote(stem+".pid") + `)" 2>/dev/null; then printf 'dead\n'; else printf 'waiting\n'; fi; tail -c 16384 ` + shellQuote(stem+".log") + ` 2>/dev/null; exit 0`
	consecutiveFailures := 0
	lastLogs := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !clock.Now().Before(deadline) {
			return fmt.Errorf("server readiness timed out; inspect the retained logs and resume to retry")
		}
		readCtx, stop := context.WithTimeout(ctx, 15*time.Second)
		output, err := launcher.Runner.Output(readCtx, strictSSHCommand(ssh, command))
		readErr := readCtx.Err() // capture before cancel; CommandContext may report only signal: killed
		stop()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil && readErr != nil {
			err = readErr
		}
		if err != nil {
			consecutiveFailures++
			if consecutiveFailures < 6 && transientReadinessRead(err, readErr) {
				delay := min(time.Duration(consecutiveFailures)*2*time.Second, 10*time.Second)
				if launcher.OnLogs != nil {
					message := fmt.Sprintf("SSH status temporarily unavailable; retry %d/5 in %ds. Remote deployment has not been restarted.", consecutiveFailures, int(delay.Seconds()))
					launcher.OnLogs(ServerLogs{Text: strings.TrimSpace(lastLogs + "\n" + message), CheckedAt: clock.Now(), Download: ParseDownloadProgress(lastLogs)})
				}
				if err := clock.Sleep(ctx, delay); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("read server logs/readiness over SSH: %w", err)
		}
		consecutiveFailures = 0
		state, raw, ok := strings.Cut(string(output), "\n")
		if !ok || (state != "ready" && state != "waiting" && state != "dead") {
			return fmt.Errorf("invalid server readiness response")
		}
		lastLogs = cleanServerLogs(raw, launcher.LogSecrets)
		if launcher.OnLogs != nil {
			text := lastLogs
			launcher.OnLogs(ServerLogs{Text: text, CheckedAt: clock.Now(), Download: ParseDownloadProgress(text)})
		}
		if state == "ready" {
			return nil
		}
		if state == "dead" {
			if cause := outOfMemoryCause(cleanServerLogs(raw, launcher.LogSecrets)); cause != "" && oomAdvice != "" {
				return fmt.Errorf("inference server exited before readiness: %s; %s", cause, oomAdvice)
			}
			return fmt.Errorf("inference server exited before readiness; inspect the retained server logs")
		}
		if err := clock.Sleep(ctx, 2*time.Second); err != nil {
			return err
		}
	}
}

func transientReadinessRead(err, readErr error) bool {
	message := strings.ToLower(err.Error())
	// Deny security failures before considering generic transport wording.
	for _, fatal := range []string{"host key", "host identification", "fingerprint", "permission denied", "authentication", "bad permissions", "identity file"} {
		if strings.Contains(message, fatal) {
			return false
		}
	}
	if errors.Is(readErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	for _, transient := range []string{"connection reset", "connection closed", "connection timed out", "connection refused", "broken pipe", "operation timed out", "no route to host"} {
		if strings.Contains(message, transient) {
			return true
		}
	}
	return false
}

func outOfMemoryCause(logs string) string {
	lines := strings.Split(logs, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		lower := strings.ToLower(lines[index])
		if strings.Contains(lower, "out of memory") || strings.Contains(lower, "cudamalloc failed") {
			return strings.TrimSpace(lines[index])
		}
	}
	return ""
}
