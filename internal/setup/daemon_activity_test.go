package setup

import (
	"context"
	"errors"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"strings"
	"testing"
	"time"
)

type daemonVastAPI struct {
	*fakeVastAPI
	calls int
	err   error
	t     *testing.T
}

func (a *daemonVastAPI) GetDaemonLogs(ctx context.Context, id int) (string, error) {
	a.calls++
	deadline, ok := ctx.Deadline()
	if id != 987 || !ok || time.Until(deadline) > 3*time.Second {
		a.t.Fatalf("unbounded or wrong logs request")
	}
	return "Pull complete secret-token\x1b[2J\nmore", a.err
}

func TestDaemonActivityIsOptionalBoundedAndNeverReadiness(t *testing.T) {
	for _, logErr := range []error{nil, errors.New("unavailable")} {
		api := &daemonVastAPI{fakeVastAPI: &fakeVastAPI{instances: []vast.Instance{{ID: 987, Status: "loading"}, {ID: 987, Status: "loading"}, {ID: 987, Status: "loading"}, {ID: 987, Status: "running", SSHHost: "host", SSHPort: 22}}}, err: logErr, t: t}
		options := testOptions()
		options.PollInterval = 15 * time.Second
		options.PollTimeout = time.Minute
		var snapshots []vast.Instance
		got, err := waitForRunning(context.Background(), api, 987, options, &fakeClock{now: time.Unix(0, 0)}, func(i vast.Instance) { snapshots = append(snapshots, i) })
		if err != nil || got.Status != "running" || api.calls != 2 || len(snapshots) != 4 {
			t.Fatalf("got=%v err=%v calls=%d", got, err, api.calls)
		}
		if logErr == nil && snapshots[0].DaemonLogs == "" {
			t.Fatal("missing daemon detail")
		}
		if logErr != nil && snapshots[0].DaemonLogs != "" {
			t.Fatal("error leaked as detail")
		}
		if snapshots[3].DaemonLogs != "" {
			t.Fatal("stale loading detail on running state")
		}
	}
}

func TestDaemonActivitySkipsTerminalAndSanitizesDetail(t *testing.T) {
	api := &daemonVastAPI{fakeVastAPI: &fakeVastAPI{instances: []vast.Instance{{ID: 987, Status: "offline"}}}, t: t}
	_, err := waitForRunning(context.Background(), api, 987, testOptions(), &fakeClock{now: time.Unix(0, 0)}, func(vast.Instance) {})
	if err == nil || api.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, api.calls)
	}
	observer := &activityOperator{fakeOperator: &fakeOperator{}}
	notifyActivity(observer, vast.Instance{Status: "loading", StatusMessage: "provider message", DaemonLogs: "Pulling secret-token\x1b[2J\nlayer"}, 987, time.Now(), "secret-token")
	got := observer.activity[0]
	if got.Message != "provider message" || !strings.Contains(got.Detail, "Pulling") || strings.Contains(got.Detail, "\x1b") || strings.Contains(got.Detail, "secret-token") {
		t.Fatalf("unsafe or lost detail: %+v", got)
	}
}

func TestDaemonActivityRetainsLatestLinesAfterLongHistory(t *testing.T) {
	observer := &activityOperator{fakeOperator: &fakeOperator{}}
	for _, latest := range []string{"Downloading layer", "Extracting layer"} {
		raw := strings.Repeat("old history "+strings.Repeat("x", 600)+"\n", 45) + latest + " secret-token\x1b[2J\nlast token=other-secret"
		notifyActivity(observer, vast.Instance{Status: "loading\n", StatusMessage: "provider\nmessage", DaemonLogs: raw}, 987, time.Now(), "secret-token")
	}
	for i, latest := range []string{"Downloading layer", "Extracting layer"} {
		got := observer.activity[i]
		lines := strings.Split(got.Detail, "\n")
		if len(lines) != 40 || lines[len(lines)-2] != latest+" [redacted]" || lines[len(lines)-1] != "last token=[redacted]" {
			t.Fatalf("lost latest sanitized lines: %q", got.Detail)
		}
		if got.Status != "loading" || got.Message != "provider message" || strings.Contains(got.Detail, "secret-token") || strings.Contains(got.Detail, "\x1b") {
			t.Fatalf("unsafe activity: %+v", got)
		}
	}
	if observer.activity[0].Detail == observer.activity[1].Detail {
		t.Fatal("changed tail did not propagate")
	}
}
