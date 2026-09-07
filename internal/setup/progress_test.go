package setup

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/maximilienGilet/sovereign-kit/internal/vast"
)

type observingOperator struct {
	*fakeOperator
	events *[]string
}

func (o *observingOperator) SetupProgress(p Progress) {
	*o.events = append(*o.events, fmt.Sprintf("%s:%d", p.Stage, p.InstanceID))
}

type observingAPI struct {
	*fakeVastAPI
	events    *[]string
	createErr error
}

func (a *observingAPI) SearchOffers(ctx context.Context, r vast.SearchRequest) ([]vast.Offer, error) {
	*a.events = append(*a.events, "search-call")
	return a.fakeVastAPI.SearchOffers(ctx, r)
}
func (a *observingAPI) CreateInstance(ctx context.Context, id int, r vast.CreateRequest) (int, error) {
	*a.events = append(*a.events, "create-call")
	if a.createErr != nil {
		return 0, a.createErr
	}
	return a.fakeVastAPI.CreateInstance(ctx, id, r)
}

// Catch notifications sent before approval, after a failed create, or too late to retain the paid ID.
func TestRunVastProgressAtOperationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name                                string
		decline, createFailure, postFailure bool
		want                                []string
	}{
		{name: "declined cost", decline: true, want: []string{"searching:0", "search-call"}},
		{name: "create failed", createFailure: true, want: []string{"searching:0", "search-call", "creating:0", "create-call"}},
		{name: "post-create failed", postFailure: true, want: []string{"searching:0", "search-call", "creating:0", "create-call", "created:987", "waiting:987"}},
		{name: "success", want: []string{"searching:0", "search-call", "creating:0", "create-call", "created:987", "waiting:987", "host-keys:987", "launching:987", "saving:987"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api, operator, _, _, _, _, deps := successfulSetup()
			events := []string{}
			observedAPI := &observingAPI{fakeVastAPI: api, events: &events}
			if tc.createFailure {
				observedAPI.createErr = errors.New("create failed")
			}
			if tc.postFailure {
				api.instances = nil
			}
			operator.confirmCost = !tc.decline
			deps.Operator = &observingOperator{fakeOperator: operator, events: &events}
			deps.NewAPI = func(string) VastAPI { return observedAPI }
			_, err := RunVast(context.Background(), "secret-token", validRecipe(), testOptions(), deps)
			if (err != nil) != (tc.decline || tc.createFailure || tc.postFailure) {
				t.Fatalf("error=%v", err)
			}
			if tc.postFailure && !strings.Contains(err.Error(), "instance 987 was created; billing may still be active") {
				t.Fatalf("missing paid warning: %v", err)
			}
			if !reflect.DeepEqual(events, tc.want) {
				t.Fatalf("events=%v want=%v", events, tc.want)
			}
		})
	}
}
