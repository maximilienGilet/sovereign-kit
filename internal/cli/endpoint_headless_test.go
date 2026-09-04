package cli

import (
	"bytes"
	"context"
	"github.com/maximilienGilet/sovereign-kit/internal/clientprofile"
	"github.com/maximilienGilet/sovereign-kit/internal/config"
	"io"
	"strings"
	"testing"
)

func TestHeadlessPrintsActualEndpointAndModelWithoutLaunching(t *testing.T) {
	tunnel := &headlessTunnel{done: make(chan error, 1)}
	tunnel.done <- nil
	// Deliver termination only after model discovery, so Connect can verify the route.
	tunnel.done = make(chan error, 1)
	var out bytes.Buffer
	err := StartHeadless(context.Background(), &out, startTestConfig(t), StartDependencies{NewTunnel: func(context.Context, config.Config, io.Writer) (Tunnel, error) { return tunnel, nil }, Healthcheck: func(context.Context, string) error { return nil }, Discover: func(_ context.Context, base string, _ clientprofile.Metadata) clientprofile.Endpoint {
		tunnel.done <- nil
		return clientprofile.Endpoint{BaseURL: base, Metadata: clientprofile.Metadata{ID: "owner/served-model"}}
	}})
	if err == nil || !strings.Contains(out.String(), "http://127.0.0.1:30000/v1") || !strings.Contains(out.String(), "owner/served-model") {
		t.Fatalf("missing endpoint guidance: %v %s", err, out.String())
	}
}
