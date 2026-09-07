package setup

import (
	"context"
	"github.com/maximilienGilet/sovereign-kit/internal/vast"
	"strings"
	"testing"
)

type activityOperator struct {
	*fakeOperator
	activity []Activity
}

func (o *activityOperator) InstanceActivity(a Activity) { o.activity = append(o.activity, a) }

func TestProvisioningPublishesProviderActivityWithoutPrematureReadiness(t *testing.T) {
	api, op, _, _, _, _, deps := successfulSetup()
	api.instances = []vast.Instance{
		{ID: 987, Status: "loading", StatusMessage: "layer: Verifying Checksum secret-token\x1b[2J"},
		{ID: 987, Status: "loading", StatusMessage: "layer: Pull complete"},
		{ID: 987, Status: "offline", StatusMessage: "Host unavailable"},
	}
	observer := &activityOperator{fakeOperator: op}
	deps.Operator = observer
	_, err := RunVast(context.Background(), "secret-token", validRecipe(), testOptions(), deps)
	if err == nil || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("missing offline error: %v", err)
	}
	if len(observer.activity) != 3 {
		t.Fatalf("missing poll activity: %+v", observer.activity)
	}
	if got := observer.activity[0].Message; !strings.Contains(got, "Verifying Checksum") || strings.Contains(got, "secret-token") || strings.Contains(got, "\x1b") {
		t.Fatalf("unsafe message: %q", got)
	}
	if observer.activity[1].Status != "loading" || observer.activity[2].Status != "offline" {
		t.Fatal("message manufactured readiness")
	}
	if observer.activity[0].CheckedAt.IsZero() || observer.activity[0].InstanceID != 987 {
		t.Fatal("activity lacks identity/time")
	}
}
