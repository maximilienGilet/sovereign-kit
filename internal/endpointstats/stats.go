// Package endpointstats reads optional server measurements through the existing
// loopback tunnel. Unsupported/missing measurements never become measured zero.
package endpointstats

import (
	"context"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Snapshot struct {
	CheckedAt                                                  time.Time
	Active, Queued                                             *float64
	DecodeTokensPerSecond, PromptTokensPerSecond, KVUsageRatio *float64
	Problem                                                    string
}

func Read(ctx context.Context, baseURL string) Snapshot {
	result := Snapshot{CheckedAt: time.Now()}
	unavailable := func(problem string) Snapshot { result.Problem = problem; return result }
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return unavailable("Activity unavailable: invalid local endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return unavailable("Activity unavailable: loopback endpoint required")
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1") + "/metrics"
	u.RawPath = ""
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return unavailable("Activity unavailable: invalid request")
	}
	// Never use a configured outbound proxy for private loopback traffic.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return unavailable("Activity unavailable: server did not respond")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return unavailable("Activity not reported by this server")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 262145))
	if err != nil || len(body) > 262144 {
		return unavailable("Activity unavailable: invalid metrics response")
	}
	values := map[string]float64{}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		n, err := strconv.ParseFloat(fields[1], 64)
		if err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 {
			values[fields[0]] = n
		}
	}
	measured := func(name string) *float64 {
		if n, ok := values["llamacpp:"+name]; ok {
			return &n
		}
		return nil
	}
	result.Active = measured("requests_processing")
	result.Queued = measured("requests_deferred")
	result.DecodeTokensPerSecond = measured("predicted_tokens_seconds")
	result.PromptTokensPerSecond = measured("prompt_tokens_seconds")
	result.KVUsageRatio = measured("kv_cache_usage_ratio")
	if result.KVUsageRatio != nil && *result.KVUsageRatio > 1 {
		result.KVUsageRatio = nil
	}
	if result.Active == nil && result.Queued == nil && result.DecodeTokensPerSecond == nil && result.PromptTokensPerSecond == nil && result.KVUsageRatio == nil {
		result.Problem = "Activity not reported by this server"
	}
	return result
}
