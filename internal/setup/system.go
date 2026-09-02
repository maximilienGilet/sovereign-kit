package setup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
)

type Command struct {
	Name  string
	Args  []string
	Stdin []byte
}

type CommandRunner interface {
	Output(context.Context, Command) ([]byte, error)
	Run(context.Context, Command) error
}

type ExecRunner struct{}

func (ExecRunner) Output(ctx context.Context, command Command) ([]byte, error) {
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	if len(command.Stdin) > 0 {
		process.Stdin = bytes.NewReader(command.Stdin)
	}
	var stderr bytes.Buffer
	process.Stderr = &stderr
	output, err := process.Output()
	if err != nil {
		return nil, fmt.Errorf("%s failed: %w", command.Name, err)
	}
	return output, nil
}

func (ExecRunner) Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	if len(command.Stdin) > 0 {
		process.Stdin = bytes.NewReader(command.Stdin)
	}
	process.Stdout = io.Discard
	process.Stderr = io.Discard
	if err := process.Run(); err != nil {
		return fmt.Errorf("%s failed: %w", command.Name, err)
	}
	return nil
}

type SystemHostKeyScanner struct {
	Runner CommandRunner
}

func (scanner SystemHostKeyScanner) Scan(ctx context.Context, host string, port int) (HostKeys, error) {
	if scanner.Runner == nil {
		return HostKeys{}, fmt.Errorf("host-key command runner is required")
	}
	if strings.TrimSpace(host) == "" || strings.ContainsAny(host, " \t\r\n") {
		return HostKeys{}, fmt.Errorf("host is required and cannot contain whitespace")
	}
	if port < 1 || port > 65535 {
		return HostKeys{}, fmt.Errorf("port must be between 1 and 65535")
	}
	raw, err := scanner.Runner.Output(ctx, Command{Name: "ssh-keyscan", Args: []string{"-T", "10", "-p", strconv.Itoa(port), host}})
	if err != nil {
		return HostKeys{}, fmt.Errorf("scan host keys: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return HostKeys{}, fmt.Errorf("scan returned blank host keys")
	}
	fingerprintBytes, err := scanner.Runner.Output(ctx, Command{Name: "ssh-keygen", Args: []string{"-lf", "-", "-E", "sha256"}, Stdin: raw})
	if err != nil {
		return HostKeys{}, fmt.Errorf("compute host-key fingerprint: %w", err)
	}
	lines := nonBlankLines(fingerprintBytes)
	if len(lines) == 0 {
		return HostKeys{}, fmt.Errorf("fingerprint output is blank")
	}
	return HostKeys{Raw: raw, Fingerprints: lines}, nil
}

func nonBlankLines(data []byte) []string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

type FileTrustStore struct{}

func (FileTrustStore) Save(path string, contents []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("known-hosts path is required")
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create known-hosts directory: %w", err)
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !bytes.Equal(existing, contents) {
			return fmt.Errorf("known-hosts file changed; refusing overwrite")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("protect known-hosts file: %w", err)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read known-hosts file: %w", err)
	}

	temporary, err := os.CreateTemp(directory, ".known_hosts.*")
	if err != nil {
		return fmt.Errorf("create known-hosts temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect known-hosts temporary file: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write known-hosts file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close known-hosts temporary file: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		if existing, readErr := os.ReadFile(path); readErr == nil && !bytes.Equal(existing, contents) {
			return fmt.Errorf("known-hosts file changed; refusing overwrite")
		}
		return fmt.Errorf("install known-hosts file: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("protect known-hosts file: %w", err)
	}
	return nil
}

func ValidateIdentityFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("identity file path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat identity file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("identity file must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("identity file is not readable: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close identity file: %w", err)
	}
	return nil
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}

func (RealClock) Sleep(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type StrictSSHLauncher struct {
	Runner CommandRunner
}

func (launcher StrictSSHLauncher) Launch(ctx context.Context, ssh config.SSH, r recipe.Recipe) error {
	if launcher.Runner == nil {
		return fmt.Errorf("server command runner is required")
	}
	if r.Runtime.Engine != "sglang" {
		return fmt.Errorf("strict SSH launcher only supports SGLang recipes")
	}
	if err := validateSSH(ssh); err != nil {
		return err
	}
	remote, err := ControlledSGLangCommand(r)
	if err != nil {
		return err
	}
	command := Command{Name: "ssh", Args: []string{
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", "UserKnownHostsFile=" + ssh.KnownHostsFile,
		"-i", ssh.IdentityFile,
		"-p", strconv.Itoa(ssh.Port),
		"--", ssh.User + "@" + ssh.Host,
		remote,
	}}
	if err := launcher.Runner.Run(ctx, command); err != nil {
		return fmt.Errorf("launch SGLang over SSH: %w", err)
	}
	return nil
}

func validateSSH(ssh config.SSH) error {
	if strings.TrimSpace(ssh.Host) == "" || strings.ContainsAny(ssh.Host, " \t\r\n") {
		return fmt.Errorf("SSH host is required and cannot contain whitespace")
	}
	if ssh.Port < 1 || ssh.Port > 65535 {
		return fmt.Errorf("SSH port must be between 1 and 65535")
	}
	if strings.TrimSpace(ssh.User) == "" || strings.ContainsAny(ssh.User, " \t\r\n") {
		return fmt.Errorf("SSH user is required and cannot contain whitespace")
	}
	if strings.TrimSpace(ssh.IdentityFile) == "" || strings.TrimSpace(ssh.KnownHostsFile) == "" {
		return fmt.Errorf("SSH identity and known-hosts paths are required")
	}
	return nil
}

func ControlledSGLangCommand(r recipe.Recipe) (string, error) {
	if r.Runtime.Engine != "sglang" {
		return "", fmt.Errorf("strict SSH launcher only supports SGLang recipes")
	}
	args := []string{
		"sglang", "serve",
		"--trust-remote-code",
		"--model-path", r.Model.Repository,
		"--revision", r.Model.Revision,
		"--context-length", strconv.Itoa(r.Serve.ContextWindow),
		"--kv-cache-dtype", "fp8_e4m3",
		"--mem-fraction-static", "0.85",
		"--attention-backend", "flashinfer",
		"--chunked-prefill-size", "2048",
		"--max-running-requests", strconv.Itoa(r.Serve.MaxRunningRequests),
		"--cuda-graph-max-bs", strconv.Itoa(r.Serve.MaxRunningRequests),
		"--reasoning-parser", "qwen3",
		"--tool-call-parser", "qwen3_coder",
		"--host", "127.0.0.1",
		"--port", "30000",
	}
	var builder strings.Builder
	builder.WriteString("nohup ")
	for i, arg := range args {
		if i > 0 {
			builder.WriteByte(' ')
		}
		if i < 2 || strings.HasPrefix(arg, "--") {
			builder.WriteString(arg)
		} else {
			builder.WriteString(shellQuote(arg))
		}
	}
	builder.WriteString(" >/workspace/sovkit-sglang.log 2>&1 </dev/null &")
	return builder.String(), nil
}
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
