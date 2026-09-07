package cli

import (
	"context"
	"errors"
	tea "github.com/charmbracelet/bubbletea"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type restartAPI struct {
	states []vast.Instance
	starts int
}

func TestQueuedRestartPollsWithoutResubmission(t *testing.T) {
	for _, existing := range []bool{false, true} {
		posts, gets := 0, 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "PUT" {
				posts++
				io.WriteString(w, `{"success":false,"error":"resources_unavailable","msg":"Required resources are currently unavailable, state change queued."}`)
				return
			}
			gets++
			if gets >= 5 {
				io.WriteString(w, `{"instances":{"id":12,"actual_status":"running","intended_status":"running","ssh_host":"fresh","ssh_port":123}}`)
				return
			}
			if existing || posts > 0 {
				io.WriteString(w, `{"instances":{"id":12,"actual_status":"exited","intended_status":"running"}}`)
			} else {
				io.WriteString(w, `{"instances":{"id":12,"actual_status":"exited","intended_status":"stopped"}}`)
			}
		}))
		cfg := config.VastStudio(12, "old", 22, "key", "hosts")
		cfg.RestartPending = existing
		statuses := []string{}
		restored := false
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := prepareReconnect(ctx, cfg, vast.NewClient(server.URL, "test"), func(context.Context, int) (bool, error) {
			if existing {
				t.Fatal("already queued request asked for restart")
			}
			return true, nil
		}, func(s string) { statuses = append(statuses, s) }, func(_ context.Context, c config.Config) (config.Config, error) { restored = true; return c, nil }, time.Millisecond)
		cancel()
		server.Close()
		wantPosts := 1
		if existing {
			wantPosts = 0
		}
		if err != nil || posts != wantPosts || !restored || !strings.Contains(strings.Join(statuses, " "), "Waiting for Vast resources") {
			t.Fatalf("existing=%v posts=%d restored=%v err=%v statuses=%v", existing, posts, restored, err, statuses)
		}
	}
}

func TestRestartConfirmationOwnsWorkerAndNeverOpensTunnelBeforeApproval(t *testing.T) {
	for _, approve := range []bool{false, true} {
		path := startTestConfig(t)
		cfg, err := config.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Provider = config.Provider{Kind: "vast", InstanceID: 12}
		if err := config.Save(path, cfg); err != nil {
			t.Fatal(err)
		}
		var connections atomic.Int32
		tunnel := &applicationTunnel{done: make(chan error, 1)}
		m := newApplication(context.Background(), path, "root", "start", ApplicationDependencies{Start: StartDependencies{
			PrepareConnection: func(ctx context.Context, cfg config.Config, confirm reconnectConfirmation, _ func(string)) (config.Config, error) {
				ok, err := confirm(ctx, 12)
				if err != nil {
					return cfg, err
				}
				if !ok {
					return cfg, context.Canceled
				}
				return cfg, nil
			},
			NewTunnel: func(context.Context, config.Config, io.Writer) (Tunnel, error) {
				connections.Add(1)
				return tunnel, nil
			},
			Healthcheck: func(context.Context, string) error { return nil },
		}})
		t.Cleanup(m.cleanup)
		first := m.beginConnection()
		prompt, ok := first().(applicationReconnectMsg)
		if !ok {
			t.Fatal("no restart confirmation")
		}
		next := m.reconnectMessage(prompt)
		if connections.Load() != 0 || m.selected || m.screen != "restart-confirm" || !strings.Contains(m.View(), "GPU billing") {
			t.Fatal("unsafe or missing default confirmation")
		}
		if !approve {
			stop := m.restartKey(tea.KeyMsg{Type: tea.KeyEnter})
			m.connectionMessage(stop().(applicationConnectionMsg))
			if connections.Load() != 0 || m.connection != nil {
				t.Fatal("cancel started tunnel or leaked worker")
			}
			continue
		}
		m.restartKey(tea.KeyMsg{Type: tea.KeyDown})
		m.restartKey(tea.KeyMsg{Type: tea.KeyEnter})
		result := next().(applicationConnectionMsg)
		if result.err != nil || connections.Load() != 1 {
			t.Fatalf("approved restart did not connect: %v", result.err)
		}
		m.connectionMessage(result)
		if m.screen != "dashboard" {
			t.Fatal("no dashboard after restart")
		}
	}
}

func TestRunningReconnectNeverRequestsRestart(t *testing.T) {
	api := &restartAPI{states: []vast.Instance{{ID: 12, Status: "running", SSHHost: "fresh", SSHPort: 124}}}
	cfg, err := prepareReconnect(context.Background(), config.VastStudio(12, "old", 22, "key", "hosts"), api, func(context.Context, int) (bool, error) {
		t.Fatal("running instance requested restart")
		return false, nil
	}, func(string) {}, func(context.Context, config.Config) (config.Config, error) {
		t.Fatal("running server relaunched")
		return config.Config{}, nil
	}, time.Millisecond)
	if err != nil || cfg.SSH.Host != "fresh" || api.starts != 0 {
		t.Fatalf("bad running reconnect: %v", err)
	}
}

func TestInterruptedRestartRestoresServerWithoutSecondPaidStart(t *testing.T) {
	api := &restartAPI{states: []vast.Instance{{ID: 12, Status: "running", SSHHost: "fresh", SSHPort: 124}}}
	cfg := config.VastStudio(12, "old", 22, "key", "hosts")
	cfg.RestartPending = true
	restored := false
	_, err := prepareReconnect(context.Background(), cfg, api, func(context.Context, int) (bool, error) {
		t.Fatal("already running instance requested a second start")
		return false, nil
	}, func(string) {}, func(_ context.Context, cfg config.Config) (config.Config, error) { restored = true; return cfg, nil }, time.Millisecond)
	if err != nil || !restored || api.starts != 0 {
		t.Fatalf("interrupted restart not restored: %v", err)
	}
}

func TestRestartWaitIsCancellableBeforeSSH(t *testing.T) {
	api := &restartAPI{states: []vast.Instance{{ID: 12, Status: "loading"}}}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := prepareReconnect(ctx, config.VastStudio(12, "old", 22, "key", "hosts"), api, nil, func(string) { cancel() }, nil, time.Millisecond)
	if !errors.Is(err, context.Canceled) || api.starts != 0 {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func (a *restartAPI) GetInstance(context.Context, int) (vast.Instance, error) {
	s := a.states[0]
	if len(a.states) > 1 {
		a.states = a.states[1:]
	}
	return s, nil
}
func (a *restartAPI) StartInstance(context.Context, int) error { a.starts++; return nil }

func TestReconnectStoppedRequiresConsentBeforeRestartAndRefreshesRoute(t *testing.T) {
	for _, approve := range []bool{false, true} {
		cfg := config.VastStudio(12, "old", 22, "identity", "known")
		api := &restartAPI{states: []vast.Instance{{ID: 12, Status: "exited"}, {ID: 12, Status: "exited"}, {ID: 12, Status: "loading"}, {ID: 12, Status: "running", SSHHost: "new", SSHPort: 123}}}
		prompted := false
		restored := false
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		got, err := prepareReconnect(ctx, cfg, api, func(context.Context, int) (bool, error) {
			if api.starts != 0 {
				t.Fatal("started before consent")
			}
			prompted = true
			return approve, nil
		}, func(string) {}, func(_ context.Context, c config.Config) (config.Config, error) { restored = true; return c, nil }, time.Millisecond)
		if !prompted {
			t.Fatal("stopped instance bypassed confirmation")
		}
		if !approve {
			if !errors.Is(err, context.Canceled) || api.starts != 0 || restored {
				t.Fatalf("decline mutated instance: %v", err)
			}
			continue
		}
		if err != nil || api.starts != 1 || !restored || got.SSH.Host != "new" || got.SSH.Port != 123 {
			t.Fatalf("restart incomplete: %+v %v", got, err)
		}
	}
}

func TestReconnectRejectsMismatchedIdentityAndOfflineWithoutStarting(t *testing.T) {
	for _, s := range []vast.Instance{{ID: 13, Status: "exited"}, {ID: 12, Status: "offline"}} {
		api := &restartAPI{states: []vast.Instance{s}}
		_, err := prepareReconnect(context.Background(), config.VastStudio(12, "old", 22, "key", "hosts"), api, func(context.Context, int) (bool, error) { t.Fatal("unexpected consent"); return true, nil }, func(string) {}, nil, time.Millisecond)
		if err == nil || api.starts != 0 {
			t.Fatal("unsafe state accepted")
		}
	}
}
