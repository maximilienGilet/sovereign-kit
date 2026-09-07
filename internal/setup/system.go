package setup

import (
	"bytes"
	"context"
	"errors"
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
	"github.com/maximilienGilet/sovereign-kit/internal/sshkey"
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

type SSHKeyRegistry interface {
	HasSSHKey(context.Context, string) (bool, error)
	AddSSHKey(context.Context, string) error
}

type IdentityOperator interface {
	ConfirmIdentitySetup(context.Context, string, bool) (bool, error)
}

func PrepareVastIdentity(ctx context.Context, path string, registry SSHKeyRegistry, runner CommandRunner, operator IdentityOperator) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("identity file path is required")
	}
	if registry == nil || runner == nil || operator == nil {
		return fmt.Errorf("Vast identity setup dependencies are required")
	}
	info, err := os.Stat(path)
	generate := errors.Is(err, os.ErrNotExist)
	if err != nil && !generate {
		return fmt.Errorf("stat identity file: %w", err)
	}
	confirmedMutation := false
	if generate {
		if _, err := os.Stat(path + ".pub"); err == nil {
			return fmt.Errorf("refusing to overwrite existing public key: %s.pub", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat public key: %w", err)
		}
		confirmed, err := operator.ConfirmIdentitySetup(ctx, path, true)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("Vast SSH identity setup cancelled")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create SSH directory: %w", err)
		}
		if err := runner.Run(ctx, Command{Name: "ssh-keygen", Args: []string{"-q", "-t", "ed25519", "-N", "", "-f", path}}); err != nil {
			return fmt.Errorf("generate SSH identity: %w", err)
		}
		confirmedMutation = true
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("identity file must be a regular file")
	}
	if err := ValidateIdentityFile(path); err != nil {
		return err
	}
	publicKeyBytes, err := runner.Output(ctx, Command{Name: "ssh-keygen", Args: []string{"-y", "-f", path}})
	if err != nil {
		return fmt.Errorf("derive SSH public key: %w", err)
	}
	publicKey := strings.TrimSpace(string(publicKeyBytes))
	if _, err := sshkey.Normalize(publicKey); err != nil {
		return fmt.Errorf("derived SSH public key is invalid: %w", err)
	}
	registered, err := registry.HasSSHKey(ctx, publicKey)
	if err != nil {
		return fmt.Errorf("check Vast SSH keys: %w", err)
	}
	if registered {
		return nil
	}
	if !confirmedMutation {
		confirmed, err := operator.ConfirmIdentitySetup(ctx, path, false)
		if err != nil {
			return err
		}
		if !confirmed {
			return fmt.Errorf("Vast SSH key registration cancelled")
		}
	}
	if err := registry.AddSSHKey(ctx, publicKey); err != nil {
		return fmt.Errorf("register Vast SSH key: %w", err)
	}
	return nil
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
		return nil, commandFailure(command.Name, err, stderr.Bytes(), command.Stdin)
	}
	return output, nil
}

func (ExecRunner) Run(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Name, command.Args...)
	if len(command.Stdin) > 0 {
		process.Stdin = bytes.NewReader(command.Stdin)
	}
	process.Stdout = io.Discard
	var stderr bytes.Buffer
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		return commandFailure(command.Name, err, stderr.Bytes(), command.Stdin)
	}
	return nil
}

func commandFailure(name string, err error, stderr, stdin []byte) error {
	displayed := strings.TrimSpace(string(stderr))
	for _, line := range bytes.Split(stdin, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			displayed = strings.ReplaceAll(displayed, string(line), "[redacted]")
		}
	}
	if len(displayed) > 2048 {
		displayed = displayed[:2048]
	}
	if displayed == "" {
		return fmt.Errorf("%s failed: %w", name, err)
	}
	return fmt.Errorf("%s failed: %w: stderr: %s", name, err, displayed)
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
		if ctx.Err() != nil {
			return HostKeys{}, ctx.Err()
		}
		var executable *exec.Error
		if errors.As(err, &executable) {
			return HostKeys{}, fmt.Errorf("scan host keys: %w", err)
		}
		return HostKeys{}, &hostKeyCollectionError{cause: err}
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return HostKeys{}, &hostKeyCollectionError{cause: fmt.Errorf("scan returned blank host keys")}
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

var linkTrustStoreFile = os.Link

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
	if err := linkTrustStoreFile(temporaryName, path); err != nil {
		if os.IsExist(err) {
			existing, readErr := os.ReadFile(path)
			if readErr == nil {
				if !bytes.Equal(existing, contents) {
					return fmt.Errorf("known-hosts file changed; refusing overwrite")
				}
				if chmodErr := os.Chmod(path, 0o600); chmodErr != nil {
					return fmt.Errorf("protect known-hosts file: %w", chmodErr)
				}
				return nil
			}
		}
		return fmt.Errorf("install known-hosts file: %w", err)
	}
	if err := os.Remove(temporaryName); err != nil {
		return fmt.Errorf("remove known-hosts temporary file: %w", err)
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
	Runner     CommandRunner
	Clock      Clock
	OnLogs     func(ServerLogs)
	LogSecrets []string
}

func (launcher StrictSSHLauncher) Launch(ctx context.Context, ssh config.SSH, r recipe.Recipe) error {
	if launcher.Runner == nil {
		return fmt.Errorf("server command runner is required")
	}
	if err := validateSSH(ssh); err != nil {
		return err
	}
	var remote string
	var err error
	switch r.Runtime.Engine {
	case "sglang":
		remote, err = ControlledSGLangCommand(r)
	case "vllm":
		remote, err = ControlledVLLMCommand(r)
	case "llama-cpp":
		remote, err = ControlledLlamaCommand(r)
	default:
		return fmt.Errorf("strict SSH launcher only supports SGLang, vLLM and reviewed llama.cpp recipes")
	}
	if err != nil {
		return err
	}
	if r.Runtime.Engine == "llama-cpp" {
		if err := launcher.prepareLlama(ctx, ssh, r); err != nil {
			return err
		}
	}
	command := strictSSHCommand(ssh, remote)
	if err := launcher.Runner.Run(ctx, command); err != nil {
		return fmt.Errorf("launch %s over SSH: %w", r.Runtime.Engine, err)
	}
	if r.Runtime.Engine == "llama-cpp" {
		return launcher.waitServerReadyWithAdvice(ctx, ssh, r.Runtime.Engine, llamaOOMAdvice(r))
	}
	if r.Runtime.Engine != "vllm" {
		return nil
	}
	output, err := launcher.Runner.Output(ctx, strictSSHCommand(ssh, controlledVLLMReadinessCommand()))
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			return fmt.Errorf("vLLM readiness failed: %w", err)
		}
		return fmt.Errorf("vLLM readiness failed: %w: %s", err, detail)
	}
	return nil
}

func llamaOOMAdvice(r recipe.Recipe) string {
	if r.ID == "qwen-solo-dual-max" {
		return "Dual Max Lab exceeded GPU memory; retry with Qwen Solo — Full Context or Qwen Solo — Dual"
	}
	return ""
}

func strictSSHCommand(ssh config.SSH, remote string) Command {
	return Command{Name: "ssh", Args: []string{
		"-o", "BatchMode=yes",
		"-o", "IdentitiesOnly=yes",
		"-o", "StrictHostKeyChecking=yes",
		"-o", sshkey.KnownHostsOption(ssh.KnownHostsFile),
		"-i", ssh.IdentityFile,
		"-p", strconv.Itoa(ssh.Port),
		"--", ssh.User + "@" + ssh.Host,
		remote,
	}}
}

func controlledVLLMReadinessCommand() string {
	return `attempt=0; while [ "$attempt" -lt 360 ]; do if curl --fail --silent --show-error --max-time 5 http://127.0.0.1:30000/health >/dev/null; then exit 0; fi; if [ ! -s /workspace/sovkit-vllm.pid ] || ! kill -0 "$(cat /workspace/sovkit-vllm.pid)" 2>/dev/null; then tail -n 80 /workspace/sovkit-vllm.log 2>/dev/null >&2; exit 1; fi; attempt=$((attempt + 1)); sleep 5; done; tail -n 80 /workspace/sovkit-vllm.log 2>/dev/null >&2; exit 1`
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
	args := []string{"python3", "-m", "sglang.launch_server"}
	if r.Model.TrustRemoteCode {
		args = append(args, "--trust-remote-code")
	}
	args = append(args,
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
	)
	return controlledBackgroundCommand(args, "/workspace/sovkit-sglang.log"), nil
}

func ControlledVLLMCommand(r recipe.Recipe) (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	if r.Runtime.Engine != "vllm" || r.Speculative == nil {
		return "", fmt.Errorf("strict SSH launcher requires the reviewed vLLM MTP recipe")
	}
	speculative := fmt.Sprintf(`{"method":"%s","num_speculative_tokens":%d}`, r.Speculative.Algorithm, r.Speculative.NumDraftTokens)
	args := []string{
		"vllm", "serve", r.Model.Repository,
		"--revision", r.Model.Revision,
	}
	if r.Model.TrustRemoteCode {
		args = append(args, "--trust-remote-code")
	}
	args = append(args,
		"--quantization", r.Runtime.Quantization,
		"--kv-cache-dtype", r.Runtime.KVCacheDType,
		"--gpu-memory-utilization", strconv.FormatFloat(r.Runtime.GPUMemoryUtilization, 'f', -1, 64),
		"--max-model-len", strconv.Itoa(r.Serve.ContextWindow),
		"--max-num-seqs", strconv.Itoa(r.Serve.MaxRunningRequests),
	)
	if r.Runtime.DisableAsyncScheduling {
		args = append(args, "--no-async-scheduling")
	}
	args = append(args,
		"--speculative-config", speculative,
		"--reasoning-parser", "qwen3",
		"--enable-auto-tool-choice",
		"--tool-call-parser", "qwen3_xml",
		"--structured-outputs-config", `{"reasoning_parser":"qwen3","enable_in_reasoning":false}`,
		"--host", "127.0.0.1",
		"--port", "30000",
	)
	return controlledBackgroundCommand(args, "/workspace/sovkit-vllm.log"), nil
}

func controlledBackgroundCommand(args []string, logPath string) string {
	var builder strings.Builder
	builder.WriteString("nohup ")
	for index, argument := range args {
		if index > 0 {
			builder.WriteByte(' ')
		}
		if index < 2 {
			builder.WriteString(argument)
		} else {
			builder.WriteString(shellQuote(argument))
		}
	}
	builder.WriteString(" >")
	builder.WriteString(logPath)
	builder.WriteString(" 2>&1 </dev/null & echo $! >")
	builder.WriteString(strings.TrimSuffix(logPath, ".log"))
	builder.WriteString(".pid")
	return builder.String()
}
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
