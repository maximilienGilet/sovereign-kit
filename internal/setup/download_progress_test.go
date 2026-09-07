package setup

import (
	"context"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"testing"
)

func TestParseDownloadProgress(t *testing.T) {
	for _, tt := range []struct {
		name, text string
		want       *DownloadProgress
	}{
		{"latest", "SOVKIT_DOWNLOAD {\"current\":0,\"total\":10}\nSOVKIT_DOWNLOAD {\"current\":5,\"total\":10}", &DownloadProgress{5, 10}},
		{"unknown", "SOVKIT_DOWNLOAD {\"current\":5,\"total\":0}", &DownloadProgress{5, 0}},
		{"complete", "SOVKIT_DOWNLOAD {\"current\":10,\"total\":10}", &DownloadProgress{10, 10}},
		{"exceeded", "SOVKIT_DOWNLOAD {\"current\":11,\"total\":10}", &DownloadProgress{11, 0}},
		{"negative", "SOVKIT_DOWNLOAD {\"current\":-1,\"total\":10}", nil},
		{"negative total", "SOVKIT_DOWNLOAD {\"current\":1,\"total\":-10}", nil},
		{"missing", "SOVKIT_DOWNLOAD {\"current\":1}", nil},
		{"null", "SOVKIT_DOWNLOAD {\"current\":null,\"total\":10}", nil},
		{"overflow", "SOVKIT_DOWNLOAD {\"current\":9223372036854775808,\"total\":0}", nil},
		{"fractional", "SOVKIT_DOWNLOAD {\"current\":1.5,\"total\":10}", nil},
		{"invalid latest", "SOVKIT_DOWNLOAD {\"current\":10,\"total\":10}\nSOVKIT_DOWNLOAD {broken", nil},
		{"verification", "SOVKIT_DOWNLOAD {\"current\":10,\"total\":10}\nVerifying GGUF SHA256", nil},
		{"launch", "SOVKIT_DOWNLOAD {\"current\":10,\"total\":10}\nLaunching llama-server: verified GGUF", nil},
		{"unstructured", "GGUF downloaded: 1.20 GB", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDownloadProgress(tt.text)
			if (got == nil) != (tt.want == nil) || got != nil && *got != *tt.want {
				t.Fatalf("got %+v want %+v", got, tt.want)
			}
		})
	}
}

func TestParseDownloadHighWaterRetainsMeasuredCompletionPastVerification(t *testing.T) {
	got := ParseDownloadHighWater("SOVKIT_DOWNLOAD {\"current\":6,\"total\":10}\nSOVKIT_DOWNLOAD {\"current\":10,\"total\":10}\nVerifying GGUF SHA256")
	if got == nil || *got != (DownloadProgress{Current: 10, Total: 10}) {
		t.Fatalf("got %+v", got)
	}
	if got := ParseDownloadHighWater("SOVKIT_DOWNLOAD {broken\nVerifying GGUF SHA256"); got != nil {
		t.Fatalf("invalid telemetry became progress: %+v", got)
	}
}

func TestServerSnapshotsExposeMeasuredDownload(t *testing.T) {
	for _, capture := range []bool{false, true} {
		var got ServerLogs
		launcher := StrictSSHLauncher{Runner: &fakeCommandRunner{onOutput: func(Command) ([]byte, error) {
			return []byte("ready\nSOVKIT_DOWNLOAD {\"current\":5,\"total\":10}\n"), nil
		}}, OnLogs: func(s ServerLogs) { got = s }}
		if capture {
			launcher.captureServerLogs(context.Background(), config.SSH{}, "llama-cpp")
		} else if err := launcher.waitServerReady(context.Background(), config.SSH{}, "llama-cpp"); err != nil {
			t.Fatal(err)
		}
		if got.Download == nil || *got.Download != (DownloadProgress{5, 10}) {
			t.Fatalf("snapshot missing counters: %+v", got)
		}
	}
}
