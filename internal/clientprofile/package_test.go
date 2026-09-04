//go:build unix

package clientprofile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPackageFailureIncludesBoundedSanitizedDiagnostics(t *testing.T) {
	err := runPackage(context.Background(), "/bin/sh", []string{"-c", "printf '\\033[31mpackage failed\\033[0m' >&2; exit 2"}, nil)
	if err == nil || !strings.Contains(err.Error(), "package failed") || strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("diagnostic %v", err)
	}
}
func TestPackageCancellationTerminatesProcessGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := runPackage(ctx, "/bin/sh", []string{"-c", "sleep 30 & wait"}, nil)
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("package process group did not cancel promptly")
	}
}
func TestMissingSubagentsExtensionIsNotReady(t *testing.T) {
	s, target, e := fixture(t)
	ctx := context.Background()
	if _, err := s.Install(ctx, target, e, s.Inspect(ctx, target, e)); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(target.Path, "npm/node_modules/pi-subagents/dist/extension.js"))
	if got := s.Inspect(ctx, target, e); got.State == Ready {
		t.Fatal("missing extension marked ready")
	}
}
