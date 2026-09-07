package cli

import (
	"context"
	"github.com/charmbracelet/x/ansi"
	"github.com/maximilienGilet/sovereign-kit/internal/setup"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogPhasesOverrideOldDownloadProgress(t *testing.T) {
	m := newApplication(context.Background(), filepath.Join(t.TempDir(), "config"), "root", "home", ApplicationDependencies{})
	defer m.cleanup()
	m.progressStage = setup.ProgressLaunching
	m.loader.stageID = setup.ProgressLaunching
	for i, tc := range []struct{ log, want string }{
		{"SOVKIT_DOWNLOAD {\"current\": 50, \"total\": 100}", "measured"},
		{"SOVKIT_DOWNLOAD {\"current\": 100, \"total\": 100}\nVerifying GGUF SHA256", "inspection"},
		{"Verifying GGUF SHA256\nLaunching llama-server: verified GGUF", "activation"},
	} {
		m.applyServerLogs(setup.ServerLogs{Text: tc.log, CheckedAt: time.Now().Add(time.Duration(i) * time.Second)})
		if got := m.loader.motifKind(); got != tc.want {
			t.Fatalf("got %s, want %s", got, tc.want)
		}
	}
}

func TestMotifUsesStageAndMeasuredDownload(t *testing.T) {
	for _, tc := range []struct{ stage, want string }{{setup.ProgressSearching, "scan"}, {setup.ProgressCreating, "orbit"}, {setup.ProgressWaiting, "orbit"}, {setup.ProgressHostKeys, "link"}, {setup.ProgressLaunching, "activation"}} {
		l := provisioningLoader{stageID: tc.stage}
		if l.motifKind() != tc.want {
			t.Fatalf("%s: %s", tc.stage, l.motifKind())
		}
	}
	l := provisioningLoader{stageID: setup.ProgressWaiting, providerMessage: "layer: Downloading"}
	if l.motifKind() != "intake" {
		t.Fatal("pull not recognized")
	}
	l = provisioningLoader{stageID: setup.ProgressLaunching, transferActive: true, transferTotal: 100, transferCurrent: 40}
	if l.motifKind() != "measured" {
		t.Fatal("lost measured transfer")
	}
	l.transferTotal = 0
	if l.motifKind() == "measured" {
		t.Fatal("invented determinate progress")
	}
}

func TestStageMotifsBoundedAndReducedMotion(t *testing.T) {
	for _, kind := range []string{"scan", "orbit", "intake", "link", "inspection", "activation", "waiting"} {
		a := stageMotif(kind, time.Second, false)
		if a != stageMotif(kind, 2*time.Second, false) {
			t.Fatal("no-color animation moved")
		}
		for _, line := range strings.Split(a, "\n") {
			if ansi.StringWidth(line) != 41 {
				t.Fatal("overflow")
			}
		}
		if strings.Contains(a, "%") {
			t.Fatal("invented progress")
		}
		if stageMotif(kind, time.Second, true) == stageMotif(kind, 2*time.Second, true) {
			t.Fatalf("%s not animated", kind)
		}
	}
}

func TestEveryIndeterminateStageRetainsCrystalSilhouette(t *testing.T) {
	for _, kind := range []string{"scan", "orbit", "intake", "link", "inspection", "activation", "waiting"} {
		for _, elapsed := range []time.Duration{0, time.Second, 4500 * time.Millisecond} {
			rows := strings.Split(ansi.Strip(stageMotif(kind, elapsed, true)), "\n")
			core := 0
			for y := 3; y < 16; y++ {
				for _, r := range []rune(rows[y])[12:29] {
					if r >= 0x2801 && r <= 0x28ff {
						core++
					}
				}
			}
			if core < 45 {
				t.Fatalf("%s at %s loses crystal: only %d core cells", kind, elapsed, core)
			}
		}
	}
}
