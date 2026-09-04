//go:build unix

package clientprofile

import (
	"context"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

func runPackage(ctx context.Context, name string, args, overrides []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = os.Environ()
	for _, value := range overrides {
		key := strings.SplitN(value, "=", 2)[0] + "="
		env := cmd.Env[:0]
		for _, old := range cmd.Env {
			if !strings.HasPrefix(old, key) {
				env = append(env, old)
			}
		}
		cmd.Env = append(env, value)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	output := &boundedOutput{}
	cmd.Stdout, cmd.Stderr = output, output
	if err := cmd.Run(); err != nil {
		detail := ansi.Strip(string(output.data))
		detail = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\t' || r >= 32 && r != 127 {
				return r
			}
			return -1
		}, detail)
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(detail))
	}
	return nil
}

type boundedOutput struct{ data []byte }

func (b *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 8192
	if n >= limit {
		b.data = append(b.data[:0], p[n-limit:]...)
		return n, nil
	}
	if len(b.data)+n > limit {
		b.data = b.data[len(b.data)+n-limit:]
	}
	b.data = append(b.data, p...)
	return n, nil
}
