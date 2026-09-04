package clientprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Install requires the exact inspection shown at confirmation. Managed files are
// prepared off-path; auth, sessions and other profile files are never moved.
func (s Service) Install(ctx context.Context, t Target, e Endpoint, confirmed Inspection) (Inspection, error) {
	fail := func(err error) (Inspection, error) { return Inspection{State: Incomplete, Detail: err.Error()}, err }
	if !e.Installable() {
		return fail(fmt.Errorf("verified model identity, context and output limits are required"))
	}
	fresh := s.Inspect(ctx, t, e)
	if fresh.State == Unreadable {
		return fail(fmt.Errorf("profile cannot be updated: %s", fresh.Detail))
	}
	if fresh.Fingerprint != confirmed.Fingerprint || fresh.State != confirmed.State {
		return fail(fmt.Errorf("profile changed since inspection; inspect and confirm again"))
	}
	if fresh.State == Ready {
		return fresh, nil
	}
	if t.Kind == Pi {
		if err := s.lookup("pi"); err != nil {
			return fail(fmt.Errorf("install Pi CLI separately before configuring this profile: %w", err))
		}
	}
	parent := filepath.Dir(t.Path)
	if err := safePath(parent); err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return fail(err)
	}
	stage, err := os.MkdirTemp(parent, ".sovereign-stage-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(stage)
	stageTarget := Target{Kind: t.Kind, Path: stage}
	if t.Kind == OpenCode {
		stageTarget.Path = filepath.Join(stage, filepath.Base(t.Path))
	}
	for _, name := range managed(t) {
		source := filepath.Join(root(t), name)
		if _, err := os.Lstat(source); os.IsNotExist(err) {
			continue
		}
		if err := copyTree(ctx, source, filepath.Join(stage, name)); err != nil {
			return fail(err)
		}
	}
	originals := map[string]map[string]any{}
	names := managed(t)
	if t.Kind == Pi {
		names = names[:2]
	}
	for _, name := range names {
		v, err := readObject(filepath.Join(stage, name))
		if err == nil {
			originals[name] = v
		} else if !os.IsNotExist(err) {
			return fail(err)
		}
	}
	rendered, err := render(t, e, originals)
	if err != nil {
		return fail(err)
	}
	for name, v := range rendered {
		raw, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fail(err)
		}
		if err = os.WriteFile(filepath.Join(stage, name), append(raw, '\n'), 0600); err != nil {
			return fail(err)
		}
	}
	run := s.Run
	if run == nil {
		run = runPackage
	}
	installCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if t.Kind == Pi {
		if err := checkPackages(stage); err != nil {
			for _, p := range packages {
				if err = run(installCtx, "pi", []string{"install", p}, []string{"PI_CODING_AGENT_DIR=" + stage}); err != nil {
					return fail(fmt.Errorf("package installation failed: %w", err))
				}
			}
		}
	} else if s.lookup("opencode") != nil {
		// A global package install would modify unrelated packages. Require the
		// pinned CLI separately, rather than silently expanding profile consent.
		return fail(fmt.Errorf("OpenCode CLI missing; install opencode-ai@1.18.25 separately, then retry"))
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	verified := s.Inspect(ctx, stageTarget, e)
	if verified.State != Ready {
		return fail(fmt.Errorf("staged profile verification failed: %s", verified.Detail))
	}
	// Detect edits (including dependency changes) made while package work ran.
	current := s.Inspect(ctx, t, e)
	if current.Fingerprint != fresh.Fingerprint || current.State != fresh.State {
		return fail(fmt.Errorf("profile changed during installation; nothing was published"))
	}
	if err := safePath(t.Path); err != nil {
		return fail(err)
	}
	backup, err := os.MkdirTemp(parent, ".sovereign-backup-")
	if err != nil {
		return fail(err)
	}
	if err := os.MkdirAll(root(t), 0700); err != nil {
		return fail(err)
	}
	moved := []string{}
	published := []string{}
	rollback := func(cause error) (Inspection, error) {
		for i := len(published) - 1; i >= 0; i-- {
			name := published[i]
			_ = os.Rename(filepath.Join(root(t), name), filepath.Join(stage, "failed-"+name))
		}
		for i := len(moved) - 1; i >= 0; i-- {
			name := moved[i]
			if err := os.Rename(filepath.Join(backup, name), filepath.Join(root(t), name)); err != nil {
				cause = fmt.Errorf("%w; restore %s manually from %s: %v", cause, name, backup, err)
			}
		}
		return fail(fmt.Errorf("%w (recoverable backup: %s)", cause, backup))
	}
	for _, name := range managed(t) {
		if err := ctx.Err(); err != nil {
			return rollback(err)
		}
		dest := filepath.Join(root(t), name)
		if err := safePath(dest); err != nil {
			return rollback(err)
		}
		if _, err := os.Lstat(dest); err == nil {
			if err = os.Rename(dest, filepath.Join(backup, name)); err != nil {
				return rollback(err)
			}
			moved = append(moved, name)
		} else if !os.IsNotExist(err) {
			return rollback(err)
		}
		if err := os.Rename(filepath.Join(stage, name), dest); err != nil {
			return rollback(err)
		}
		published = append(published, name)
	}
	result := s.Inspect(ctx, t, e)
	if result.State != Ready {
		return rollback(fmt.Errorf("installed profile verification failed: %s", result.Detail))
	}
	result.Backup = backup
	return result, nil
}

func copyTree(ctx context.Context, source, dest string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		rel, _ := filepath.Rel(source, path)
		target := filepath.Join(dest, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		if info.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing special file %s", path)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()&0700)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, input)
		closeErr := output.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}
