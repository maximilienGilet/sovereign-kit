package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type VastCredentials interface {
	Load() (string, error)
	Save(string) error
}

// FileVastCredentials stores a local plaintext secret separately from shareable
// configuration and checkpoints. Only the current owner may read it.
type FileVastCredentials struct{ Path string }

func (s FileVastCredentials) Load() (string, error) {
	fd, err := syscall.Open(s.Path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return "", &os.PathError{Op: "open", Path: s.Path, Err: err}
	}
	f := os.NewFile(uintptr(fd), s.Path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("Vast credential file must be owner-only (0600)")
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Uid != uint32(os.Geteuid()) {
		return "", fmt.Errorf("Vast credential file must belong to current user")
	}
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return "", err
	}
	if len(data) > 65536 {
		return "", fmt.Errorf("invalid Vast credential file")
	}
	return strings.TrimSpace(string(data)), nil
}

func (s FileVastCredentials) Save(token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 65536 {
		return fmt.Errorf("invalid Vast API key")
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".vast-key-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.WriteString(token); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), s.Path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func vastAPIKey(ctx context.Context, deps SetupDependencies) (string, error) {
	if token := strings.TrimSpace(deps.Getenv("VAST_API_KEY")); token != "" {
		return token, nil
	}
	if deps.Credentials != nil {
		token, err := deps.Credentials.Load()
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("cannot read saved Vast API key; check credential file permissions or set VAST_API_KEY")
		}
		if err == nil && token != "" {
			return token, nil
		}
	}
	token, err := deps.Prompter.VastAPIKey(ctx)
	if err != nil {
		return "", fmt.Errorf("get a Vast API key at https://console.vast.ai/keys, then retry or set VAST_API_KEY: %w", err)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("get a Vast API key at https://console.vast.ai/keys, then retry or set VAST_API_KEY")
	}
	if deps.Credentials != nil {
		if err := deps.Credentials.Save(token); err != nil {
			return "", fmt.Errorf("cannot save Vast API key securely; provisioning was not started")
		}
	}
	return token, nil
}
