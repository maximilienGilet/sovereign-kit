package setup

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReconciliationNeverRedispatchesUncertainLaunch(t *testing.T) {
	dir := t.TempDir()
	proc := filepath.Join(dir, "proc")
	if err := os.Mkdir(proc, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "deployment")
	command := "echo dispatched >> " + shellQuote(filepath.Join(dir, "calls"))
	script := reconciliationScript(validRecipe(), command, marker, proc)
	script = strings.Replace(script, "probe = socket.socket()", "probe = type('FreePort', (), {'bind': lambda self, address: None, 'close': lambda self: None})()", 1)
	run := func() error {
		output, err := exec.CommandContext(context.Background(), "python3", "-c", script).CombinedOutput()
		if err != nil {
			t.Log(string(output))
		}
		return err
	}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	if err := run(); err == nil {
		t.Fatal("uncertain dispatch must refuse retry")
	}
	data, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil || string(data) != "dispatched\n" {
		t.Fatalf("launches %q error=%v", data, err)
	}
}

func TestReconciliationCreatesMissingWorkspaceBeforeLock(t *testing.T) {
	dir := t.TempDir()
	proc := filepath.Join(dir, "proc")
	if err := os.Mkdir(proc, 0700); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(dir, "missing-workspace")
	marker := filepath.Join(workspace, ".sovkit-deployment")
	calls := filepath.Join(workspace, "calls")
	script := reconciliationScript(validRecipe(), "echo dispatched >> "+shellQuote(calls), marker, proc)
	// Only the network port probe is substituted; execute the real directory,
	// locking, checkpoint and dispatch operations in an isolated filesystem.
	script = strings.Replace(script, "probe = socket.socket()", "probe = type('FreePort', (), {'bind': lambda self, address: None, 'close': lambda self: None})()", 1)
	output, err := exec.Command("python3", "-c", script).CombinedOutput()
	if err != nil {
		t.Fatalf("fresh workspace failed: %v\n%s", err, output)
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		t.Fatal("missing private workspace")
	}
	if _, err := exec.Command("python3", "-c", script).CombinedOutput(); err == nil {
		t.Fatal("existing launch was redispatched")
	}
	data, err := os.ReadFile(calls)
	if err != nil || string(data) != "dispatched\n" {
		t.Fatalf("unexpected dispatches: %q %v", data, err)
	}
}

func TestReconciliationRejectsCompetingProcessEvenWithMatchingParent(t *testing.T) {
	dir := t.TempDir()
	proc := filepath.Join(dir, "proc")
	marker := filepath.Join(dir, "deployment")
	r := validRecipe()
	for id, args := range map[string]string{"123": "sglang\x00serve\x00" + r.Model.Repository + "\x00--revision\x00" + r.Model.Revision + "\x00", "124": "vllm\x00serve\x00foreign/model\x00"} {
		if err := os.MkdirAll(filepath.Join(proc, id), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(proc, id, "cmdline"), []byte(args), 0600); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := json.Marshal(r)
	value, _ := json.Marshal(digest(snapshot))
	if err := os.WriteFile(marker, value, 0600); err != nil {
		t.Fatal(err)
	}
	script := reconciliationScript(r, "exit 99", marker, proc)
	if err := exec.Command("python3", "-c", script).Run(); err == nil {
		t.Fatal("competing process accepted")
	}
}

func TestReconciliationRefusesForeignServer(t *testing.T) {
	dir := t.TempDir()
	proc := filepath.Join(dir, "proc")
	if err := os.MkdirAll(filepath.Join(proc, "123"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proc, "123", "cmdline"), []byte("python\x00-m\x00sglang.launch_server\x00--model-path\x00foreign/model\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	script := reconciliationScript(validRecipe(), "touch "+shellQuote(filepath.Join(dir, "launched")), filepath.Join(dir, "deployment"), proc)
	if err := exec.Command("python3", "-c", script).Run(); err == nil {
		t.Fatal("foreign server accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "launched")); !os.IsNotExist(err) {
		t.Fatal("launched alongside foreign server")
	}
}

func TestReconciliationRecoversOnlyProvenMissingSGLangExecutable(t *testing.T) {
	for _, tc := range []struct {
		name, log                         string
		wrongRecipe, occupied, wantLaunch bool
	}{
		{"missing executable", "nohup: failed to run command 'sglang': No such file or directory\n", false, false, true},
		{"other error", "CUDA out of memory\n", false, false, false},
		{"wrong recipe", "nohup: failed to run command 'sglang': No such file or directory\n", true, false, false},
		{"occupied port", "nohup: failed to run command 'sglang': No such file or directory\n", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			proc := filepath.Join(dir, "proc")
			os.Mkdir(proc, 0700)
			marker := filepath.Join(dir, "deployment")
			log := filepath.Join(dir, "sovkit-sglang.log")
			snapshot, _ := json.Marshal(validRecipe())
			fingerprint := digest(snapshot)
			if tc.wrongRecipe {
				fingerprint = "foreign"
			}
			data, _ := json.Marshal(fingerprint)
			os.WriteFile(marker, data, 0600)
			os.WriteFile(log, []byte(tc.log), 0600)
			calls := filepath.Join(dir, "calls")
			script := reconciliationScript(validRecipe(), "echo dispatched >> "+shellQuote(calls), marker, proc)
			probe := "probe = type('FreePort', (), {'bind': lambda self, address: None, 'close': lambda self: None})()"
			if tc.occupied {
				probe = "probe = type('BusyPort', (), {'bind': lambda self, address: (_ for _ in ()).throw(OSError('busy')), 'close': lambda self: None})()"
			}
			script = strings.Replace(script, "probe = socket.socket()", probe, 1)
			out, err := exec.Command("python3", "-c", script).CombinedOutput()
			if (err == nil) != tc.wantLaunch {
				t.Fatalf("unexpected recovery result: %v %s", err, out)
			}
			if tc.wantLaunch {
				archived, err := os.ReadFile(log + ".missing-command")
				if err != nil || string(archived) != tc.log {
					t.Fatal("previous failure evidence not preserved")
				}
				if err := exec.Command("python3", "-c", script).Run(); err == nil {
					t.Fatal("recovery redispatched twice")
				}
				content, _ := os.ReadFile(calls)
				if string(content) != "dispatched\n" {
					t.Fatal("wrong launch count")
				}
			} else if _, err := os.Stat(calls); !os.IsNotExist(err) {
				t.Fatal("unsafe recovery launched")
			}
		})
	}
}
