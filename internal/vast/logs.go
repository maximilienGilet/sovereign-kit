package vast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// GetDaemonLogs retrieves one optional snapshot. Uploads are asynchronous; an
// absent snapshot is not evidence that the instance has stopped progressing.
func (client *Client) GetDaemonLogs(ctx context.Context, instanceID int) (string, error) {
	if strings.TrimSpace(client.token) == "" {
		return "", fmt.Errorf("Vast API token is required")
	}
	if instanceID <= 0 {
		return "", fmt.Errorf("Vast instance ID must be positive")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Neither API redirects nor storage redirects may carry credentials elsewhere.
	httpClient := *client.http
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	httpClient.Jar = nil
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/api/v0/instances/request_logs/%d/", client.baseURL, instanceID), strings.NewReader(`{"daemon_logs":"true","tail":"40"}`))
	if err != nil {
		return "", fmt.Errorf("invalid Vast logs request")
	}
	req.Header.Set("Authorization", "Bearer "+client.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("Vast logs request unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Vast logs request returned HTTP %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(body) > 65536 {
		return "", fmt.Errorf("Vast logs response unavailable or too large")
	}
	var result struct {
		Success   bool   `json:"success"`
		ResultURL string `json:"result_url"`
	}
	if err := decodeSingleJSON(strings.NewReader(string(body)), &result); err != nil {
		return "", fmt.Errorf("invalid Vast logs response")
	}
	if !result.Success {
		return "", fmt.Errorf("Vast logs request rejected")
	}
	if result.ResultURL == "" {
		return "", nil
	}
	u, err := url.Parse(result.ResultURL)
	// Accept the documented bucket and the public.vast.ai bucket observed in live
	// API responses. Unknown destinations fail closed; never become a URL fetcher.
	if err != nil || u.Scheme != "https" || u.Host != "s3.amazonaws.com" || u.User != nil || u.Fragment != "" || !(strings.HasPrefix(u.Path, "/vast.ai/instance_logs/") || strings.HasPrefix(u.Path, "/public.vast.ai/instance_logs/")) || path.Clean(u.Path) != u.Path || strings.Contains(u.Path, "\\") {
		return "", fmt.Errorf("unsafe Vast logs storage URL")
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("invalid Vast logs storage request")
	}
	for {
		res, err = httpClient.Do(req)
		if err != nil {
			return "", fmt.Errorf("Vast log snapshot unavailable")
		}
		if res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusAccepted {
			break
		}
		res.Body.Close()
		timer := time.NewTimer(300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", nil
		case <-timer.C:
		}
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Vast log snapshot returned HTTP %d", res.StatusCode)
	}
	body, err = io.ReadAll(io.LimitReader(res.Body, 65537))
	if err != nil || len(body) > 65536 {
		return "", fmt.Errorf("Vast log snapshot unavailable or too large")
	}
	logs := strings.ReplaceAll(string(body), client.token, "[redacted]")
	logs = strings.ReplaceAll(logs, strings.TrimSpace(client.token), "[redacted]")
	return strings.TrimSpace(logs), nil
}
