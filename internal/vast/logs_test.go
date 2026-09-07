package vast

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type logsTransport func(*http.Request) (*http.Response, error)

func (f logsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func logsResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestDaemonLogsContractAndNoCredentialForwarding(t *testing.T) {
	c := NewClient("https://console.vast.ai", "secret-token")
	calls := 0
	c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if r.Method != "PUT" || r.URL.Path != "/api/v0/instances/request_logs/987/" || r.Header.Get("Authorization") != "Bearer secret-token" {
				t.Fatalf("bad API request: %v", r)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload) != 2 || payload["daemon_logs"] != "true" || payload["tail"] != "40" {
				t.Fatalf("payload=%v", payload)
			}
			return logsResponse(200, `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/987.log?signature=private","msg":""}`), nil
		}
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.URL.Host != "s3.amazonaws.com" {
			t.Fatalf("unsafe storage request: %v", r)
		}
		return logsResponse(200, "Pulling layer secret-token\n"), nil
	})}
	got, err := c.GetDaemonLogs(context.Background(), 987)
	if err != nil || strings.Contains(got, "secret-token") || !strings.Contains(got, "Pulling layer") || calls != 2 {
		t.Fatalf("logs=%q err=%v calls=%d", got, err, calls)
	}
}

func TestDaemonLogsLivePublicBucket(t *testing.T) {
	c := NewClient("https://console.vast.ai", "secret-token")
	c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "PUT" {
			return logsResponse(200, `{"success":true,"result_url":"https://s3.amazonaws.com/public.vast.ai/instance_logs/snapshot.log"}`), nil
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("credential forwarded to storage")
		}
		return logsResponse(200, "sshd: no hostkeys available -- exiting."), nil
	})}
	got, err := c.GetDaemonLogs(context.Background(), 49874528)
	if err != nil || got != "sshd: no hostkeys available -- exiting." {
		t.Fatalf("logs=%q err=%v", got, err)
	}
}

func TestDaemonLogsRejectUnsafeStorageURLs(t *testing.T) {
	for _, url := range []string{"http://s3.amazonaws.com/vast.ai/instance_logs/test.log", "https://127.0.0.1/log", "https://169.254.169.254/log", "https://s3.amazonaws.com.evil.test/log", "https://user:pass@s3.amazonaws.com/vast.ai/instance_logs/test.log", "https://s3.amazonaws.com:444/vast.ai/instance_logs/test.log", "https://evil.test/log", "https://s3.amazonaws.com/another-bucket/log", "https://s3.amazonaws.com/vast.ai/instance_logs/../secret", "https://s3.amazonaws.com/vast.ai/instance_logs/%2e%2e/secret", "https://s3.amazonaws.com/vast.ai/instance_logs/log#fragment"} {
		t.Run(url, func(t *testing.T) {
			c := NewClient("https://console.vast.ai", "secret-token")
			calls := 0
			c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				return logsResponse(200, fmt.Sprintf(`{"success":true,"result_url":%q}`, url)), nil
			})}
			_, err := c.GetDaemonLogs(context.Background(), 987)
			if err == nil || calls != 1 || strings.Contains(err.Error(), url) {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestDaemonLogsUnavailableBoundedAndNoRedirect(t *testing.T) {
	for _, test := range []struct {
		name, api, body string
		status          int
		wantErr         bool
	}{
		{"missing", `{"success":true}`, "", 200, false},
		{"rejected", `{"success":false,"msg":"secret-token"}`, "", 200, true},
		{"delayed", `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/test.log"}`, "", 404, false},
		{"redirect", `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/test.log"}`, "", 302, true},
		{"large", `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/test.log"}`, strings.Repeat("x", 65537), 200, true},
		{"badjson", "{secret-token", "", 200, true},
		{"large API response", strings.Repeat("x", 65537), "", 200, true},
		{"accepted", `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/test.log"}`, "", 202, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewClient("https://console.vast.ai", "secret-token")
			calls := 0
			c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return logsResponse(200, test.api), nil
				}
				res := logsResponse(test.status, test.body)
				if (test.status == 404 || test.status == 202) && calls > 2 {
					res = logsResponse(200, "")
				}
				res.Header.Set("Location", "https://evil.test/")
				return res, nil
			})}
			got, err := c.GetDaemonLogs(context.Background(), 987)
			if (err != nil) != test.wantErr || got != "" || calls > 3 {
				t.Fatalf("logs=%q err=%v calls=%d", got, err, calls)
			}
			if err != nil && strings.Contains(err.Error(), "secret-token") {
				t.Fatal("secret in error")
			}
		})
	}
}

func TestDaemonLogsPollsSameAsynchronousSnapshot(t *testing.T) {
	c := NewClient("https://console.vast.ai", "secret-token")
	puts, gets := 0, 0
	c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "PUT" {
			puts++
			return logsResponse(200, `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/snapshot.log"}`), nil
		}
		gets++
		if r.URL.String() != "https://s3.amazonaws.com/vast.ai/instance_logs/snapshot.log" {
			t.Fatal("snapshot URL changed")
		}
		if gets == 1 {
			return logsResponse(404, ""), nil
		}
		return logsResponse(200, "Pulling layer"), nil
	})}
	got, err := c.GetDaemonLogs(context.Background(), 987)
	if err != nil || got != "Pulling layer" || puts != 1 || gets != 2 {
		t.Fatalf("logs=%q err=%v puts=%d gets=%d", got, err, puts, gets)
	}
}

func TestDaemonLogsAPIErrorsDoNotFollowRedirectsOrRevealBodies(t *testing.T) {
	for _, status := range []int{302, 403, 500} {
		c := NewClient("https://console.vast.ai", "secret-token")
		calls := 0
		c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
			calls++
			res := logsResponse(status, "secret-token")
			res.Header.Set("Location", "https://evil.test/")
			return res, nil
		})}
		_, err := c.GetDaemonLogs(context.Background(), 987)
		if err == nil || calls != 1 || strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("calls=%d err=%v", calls, err)
		}
	}
}

func TestDaemonLogsCancellationReachesHTTP(t *testing.T) {
	c := NewClient("https://console.vast.ai", "secret-token")
	c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 3*time.Second {
			t.Fatal("missing bounded deadline")
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.GetDaemonLogs(ctx, 987)
	if err == nil {
		t.Fatal("cancelled request succeeded")
	}
}

func TestDaemonLogsCancelsDelayedUploadWithoutAnotherRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := NewClient("https://console.vast.ai", "secret-token")
	calls := 0
	c.http = &http.Client{Transport: logsTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return logsResponse(200, `{"success":true,"result_url":"https://s3.amazonaws.com/vast.ai/instance_logs/snapshot.log"}`), nil
		}
		cancel()
		return logsResponse(404, ""), nil
	})}
	start := time.Now()
	got, _ := c.GetDaemonLogs(ctx, 987)
	if got != "" || calls != 2 || time.Since(start) > 200*time.Millisecond {
		t.Fatalf("logs=%q calls=%d elapsed=%v", got, calls, time.Since(start))
	}
}
