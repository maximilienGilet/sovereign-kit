package cli

import (
	"bytes"
	"context"
	"errors"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"strings"
	"testing"
)

func TestAccessibleRecoveryDeclineEOFAndRetry(t *testing.T) {
	for _, tc := range []struct {
		input     string
		wantCalls int
	}{{"", 0}, {"\n", 0}, {"yes", 0}, {"yes\nyes\n", 2}} {
		t.Run(tc.input, func(t *testing.T) {
			var out bytes.Buffer
			p := NewAccessiblePrompter(strings.NewReader(tc.input), &out)
			observer, ok := any(p).(setup.RecoveryObserver)
			if !ok {
				t.Fatal("accessible prompter does not preserve recovery")
			}
			calls := 0
			observer.InstanceCreated(setup.InstanceRecovery{InstanceID: 789, Destroy: func(ctx context.Context) error {
				calls++
				if ctx.Err() != nil {
					t.Error("cleanup context cancelled")
				}
				if _, ok := ctx.Deadline(); !ok {
					t.Error("unbounded cleanup")
				}
				if calls == 1 {
					return errors.New("token-secret network failure")
				}
				return nil
			}})
			handler, ok := any(p).(interface {
				recoverSetup(context.Context, error, string) error
			})
			if !ok {
				t.Fatal("missing accessible error recovery")
			}
			err := handler.recoverSetup(context.Background(), errors.New("original token-secret failure"), "token-secret")
			if err == nil {
				t.Fatal("cleanup manufactured setup success")
			}
			if calls != tc.wantCalls {
				t.Fatalf("calls=%d want=%d", calls, tc.wantCalls)
			}
			if strings.Contains(out.String()+err.Error(), "token-secret") {
				t.Fatal("leaked provider secret")
			}
			if strings.Contains(out.String(), "\x1b") {
				t.Fatal("accessible output contains ANSI")
			}
			if calls == 2 && (!strings.Contains(out.String(), "#789 destroyed") || strings.Contains(err.Error(), "billing may")) {
				t.Fatalf("stale final result: %s %v", out.String(), err)
			}
		})
	}
}

func TestAccessibleNewSetupForgetsPreviousRunCapability(t *testing.T) {
	var out bytes.Buffer
	p := NewAccessiblePrompter(strings.NewReader("1\n"), &out)
	p.InstanceCreated(setup.InstanceRecovery{InstanceID: 987, Destroy: func(context.Context) error { t.Fatal("old instance targeted"); return nil }})
	Setup(context.Background(), nil, &out, t.TempDir()+"/config", "root", SetupDependencies{Prompter: p, Getenv: func(string) string { return "" }})
	if p.recovery.InstanceID != 0 {
		t.Fatal("new setup retained prior-session destruction capability")
	}
}
