package setup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/recipe"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

// Checkpoint deliberately contains no credentials, only immutable deployment inputs.
type Checkpoint struct {
	Version        int
	InstanceID     int
	Recipe         recipe.Recipe
	IdentityFile   string
	KnownHostsDir  string
	Phase          string
	ApprovedHost   string
	ApprovedPort   int
	KnownHostsFile string
	HostKeysHash   string
}

type ServerReconciler interface {
	Reconcile(context.Context, config.SSH, recipe.Recipe) error
}

func CheckpointPath(configPath string) string { return configPath + ".pending.json" }

func ReadCheckpoint(path string) (Checkpoint, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return Checkpoint{}, &os.PathError{Op: "open", Path: path, Err: err}
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Checkpoint{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return Checkpoint{}, fmt.Errorf("checkpoint must be an owner-only regular file")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && stat.Uid != uint32(os.Geteuid()) {
		return Checkpoint{}, fmt.Errorf("checkpoint must be owned by current user")
	}
	data, err := io.ReadAll(io.LimitReader(file, 1024*1024))
	if err != nil {
		return Checkpoint{}, err
	}
	var cp Checkpoint
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cp); err != nil {
		return cp, fmt.Errorf("invalid provisioning checkpoint: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return cp, fmt.Errorf("invalid trailing checkpoint content")
	}
	if cp.Version != 1 || cp.InstanceID < 0 || strings.TrimSpace(cp.IdentityFile) == "" || strings.TrimSpace(cp.KnownHostsDir) == "" {
		return cp, fmt.Errorf("invalid or unsupported provisioning checkpoint")
	}
	switch cp.Phase {
	case "create-intent", "created", "trusted", "launch-intent":
	default:
		return cp, fmt.Errorf("unsupported checkpoint phase")
	}
	if cp.InstanceID == 0 && cp.Phase != "create-intent" {
		return cp, fmt.Errorf("checkpoint has no instance ID")
	}
	if err := cp.Recipe.Validate(); err != nil {
		return cp, fmt.Errorf("invalid checkpoint recipe: %w", err)
	}
	return cp, nil
}

func writeCheckpoint(path string, cp Checkpoint) error {
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	return syncDirectory(dir)
}

func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func removeCheckpoint(path string) error {
	if err := os.Remove(path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(path))
}

func lockCheckpoint(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	fd, err := syscall.Open(path+".lock", syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		syscall.Close(fd)
		return nil, fmt.Errorf("another provisioning or recovery operation is active: %w", err)
	}
	return func() { syscall.Flock(fd, syscall.LOCK_UN); syscall.Close(fd) }, nil
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func hostKeysDigest(raw []byte) string {
	var keys []string
	for _, line := range strings.Split(string(raw), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && !strings.HasPrefix(fields[0], "#") {
			keys = append(keys, strings.Join(fields[:3], " "))
		}
	}
	sort.Strings(keys)
	return digest([]byte(strings.Join(keys, "\n")))
}
func (cp Checkpoint) trustMatches(instance vast.Instance, path string, raw []byte) bool {
	if cp.ApprovedHost != instance.SSHHost || cp.ApprovedPort != instance.SSHPort || cp.KnownHostsFile != path || cp.HostKeysHash != hostKeysDigest(raw) {
		return false
	}
	stored, err := os.ReadFile(path)
	return err == nil && hostKeysDigest(stored) == cp.HostKeysHash
}

func runDurableVast(ctx context.Context, token string, r recipe.Recipe, options Options, deps Dependencies) (Result, error) {
	unlock, err := lockCheckpoint(options.CheckpointPath)
	if err != nil {
		return Result{}, err
	}
	defer unlock()
	cp, readErr := ReadCheckpoint(options.CheckpointPath)
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return Result{}, readErr
	}
	resume := options.Resume || options.ResumeInstanceID > 0
	if options.ResumeInstanceID < 0 || (options.RecoveryOnly && !resume) {
		return Result{}, fmt.Errorf("invalid resume options")
	}
	if exists && !resume {
		return Result{}, fmt.Errorf("unfinished provisioning exists; continue or destroy it before creating another instance")
	}
	if exists && cp.InstanceID > 0 && options.ResumeInstanceID > 0 && cp.InstanceID != options.ResumeInstanceID {
		return Result{}, fmt.Errorf("resume ID does not match checkpoint instance %d", cp.InstanceID)
	}
	if exists {
		r = cp.Recipe
		options.IdentityFile = cp.IdentityFile
		options.KnownHostsDir = cp.KnownHostsDir
	}
	if strings.TrimSpace(token) == "" {
		return Result{}, fmt.Errorf("Vast API key is required")
	}
	if err := r.Validate(); err != nil {
		return Result{}, err
	}
	if deps.NewAPI == nil || deps.Operator == nil || deps.Clock == nil || deps.ValidateIdentity == nil || deps.HostKeyScanner == nil || deps.TrustStore == nil || deps.ServerLauncher == nil || deps.SaveConfig == nil {
		return Result{}, fmt.Errorf("setup dependencies are required")
	}
	if strings.TrimSpace(options.IdentityFile) == "" || strings.TrimSpace(options.KnownHostsDir) == "" {
		return Result{}, fmt.Errorf("explicit identity and known-hosts directory are required")
	}
	api := deps.NewAPI(strings.TrimSpace(token))
	if api == nil {
		return Result{}, fmt.Errorf("Vast API is required")
	}
	if !exists {
		cp = Checkpoint{Version: 1, Recipe: r, IdentityFile: options.IdentityFile, KnownHostsDir: options.KnownHostsDir, Phase: "create-intent"}
	}
	if resume {
		if cp.InstanceID == 0 {
			if options.ResumeInstanceID <= 0 {
				return Result{}, fmt.Errorf("creation outcome is unknown; inspect Vast account and resume with an explicit instance ID; do not create again")
			}
			instance, err := api.GetInstance(ctx, options.ResumeInstanceID)
			if err != nil {
				return Result{}, err
			}
			if instance.ID != options.ResumeInstanceID || instance.Image != r.Runtime.Image {
				return Result{}, fmt.Errorf("cannot verify exact instance and pinned image compatibility for legacy resume")
			}
			cp.InstanceID, cp.Phase = options.ResumeInstanceID, "created"
			if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
				return Result{}, paidInstanceError(cp.InstanceID, err)
			}
		}
		// Exact account ownership is established before rebuilding destructive capability.
		instance, err := api.GetInstance(ctx, cp.InstanceID)
		if err != nil {
			return Result{}, paidInstanceError(cp.InstanceID, err)
		}
		if instance.ID != cp.InstanceID {
			return Result{}, fmt.Errorf("provider returned a different instance; resume refused")
		}
		if options.RecoveryOnly {
			destroyer, ok := api.(InstanceDestroyer)
			if !ok {
				return Result{}, fmt.Errorf("instance recovery is unavailable")
			}
			notifyRecovery(deps.Operator, newInstanceRecovery(cp.InstanceID, destroyer, options, deps.Clock))
			return Result{InstanceID: cp.InstanceID}, nil
		}
	} else {
		notifyProgress(deps.Operator, ProgressSearching, 0)
		selected, err := chooseEligibleOffer(ctx, api, r, options.OfferLimit, deps.Operator, token)
		if err != nil {
			return Result{}, err
		}
		if selected.Offer.PriceUnknown {
			return Result{}, fmt.Errorf("selected Vast offer has unknown price")
		}
		confirmed, err := deps.Operator.ConfirmCost(ctx, selected, r.Requirements.MinimumDiskGB)
		if err != nil {
			return Result{}, err
		}
		if !confirmed {
			return Result{}, fmt.Errorf("Vast setup cancelled: cost was not confirmed")
		}
		if deps.PrepareIdentity != nil {
			if err := deps.PrepareIdentity(ctx); err != nil {
				return Result{}, err
			}
		}
		if err := deps.ValidateIdentity(options.IdentityFile); err != nil {
			return Result{}, err
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
			return Result{}, err
		}
		notifyProgress(deps.Operator, ProgressCreating, 0)
		id, err := api.CreateInstance(ctx, selected.Offer.ID, vast.CreateRequest{Image: r.Runtime.Image, DiskGB: r.Requirements.MinimumDiskGB, Label: "sovkit-" + r.ID})
		if err != nil {
			return Result{}, fmt.Errorf("creation outcome uncertain; checkpoint retained; inspect Vast before retrying: %w", err)
		}
		if id <= 0 {
			return Result{}, fmt.Errorf("creation outcome uncertain: no positive instance ID; checkpoint retained")
		}
		cp.InstanceID, cp.Phase = id, "created"
		if destroyer, ok := api.(InstanceDestroyer); ok {
			notifyRecovery(deps.Operator, newInstanceRecovery(id, destroyer, options, deps.Clock))
		}
		if err := writeCheckpoint(options.CheckpointPath, cp); err != nil {
			return Result{}, paidInstanceError(id, err)
		}
	}
	if err := deps.ValidateIdentity(options.IdentityFile); err != nil {
		return Result{}, paidInstanceError(cp.InstanceID, err)
	}
	return finishVast(ctx, token, r, options, deps, api, cp.InstanceID, &cp)
}
