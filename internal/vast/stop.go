package vast

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// StateChangeError preserves the distinction between a queued request and a
// rejection. Neither is confirmation that the instance is already running.
type StateChangeError struct {
	message string
	queued  bool
}

func (e *StateChangeError) Error() string { return e.message }
func (e *StateChangeError) Queued() bool  { return e.queued }

// StopInstance requests a stop for one exact instance. A successful response
// acknowledges the request only; callers must verify actual_status separately.
// Contract: vast-ai/vast-python vast.py, stop_instance (PUT state=stopped).
func (client *Client) StopInstance(ctx context.Context, instanceID int) error {
	return client.setInstanceState(ctx, instanceID, "stopped")
}

// StartInstance acknowledges a restart request; actual readiness must be polled.
func (client *Client) StartInstance(ctx context.Context, instanceID int) error {
	return client.setInstanceState(ctx, instanceID, "running")
}

func (client *Client) setInstanceState(ctx context.Context, instanceID int, state string) error {
	if strings.TrimSpace(client.token) == "" {
		return fmt.Errorf("Vast API token is required")
	}
	if instanceID <= 0 {
		return fmt.Errorf("Vast instance ID must be positive")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/api/v0/instances/%d/", client.baseURL, instanceID), strings.NewReader(fmt.Sprintf(`{"state":%q}`, state)))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	var result struct {
		Success *bool  `json:"success"`
		Error   string `json:"error"`
		Message string `json:"msg"`
		Detail  string `json:"detail"`
	}
	decodeErr := decodeSingleJSON(io.LimitReader(response.Body, 64*1024), &result)
	prefix := fmt.Sprintf("Vast instance #%d state %s (HTTP %d)", instanceID, state, response.StatusCode)
	if decodeErr != nil {
		return fmt.Errorf("%s: invalid acknowledgement", prefix)
	}
	if response.StatusCode != http.StatusOK || result.Success == nil || !*result.Success {
		reason := "request rejected"
		if result.Success == nil {
			reason = "unconfirmed response: missing success"
		}
		details := strings.TrimSpace(strings.Join([]string{result.Error, result.Message, result.Detail}, " "))
		// API diagnostics are untrusted terminal text and can echo credentials.
		details = strings.ReplaceAll(details, client.token, "[redacted]")
		details = strings.ReplaceAll(details, strings.TrimSpace(client.token), "[redacted]")
		details = ansi.Strip(details)
		details = strings.Map(func(r rune) rune {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				return ' '
			}
			return r
		}, details)
		runes := []rune(details)
		if len(runes) > 512 {
			details = string(runes[:512]) + "…"
		}
		if details != "" {
			reason += " — " + details
		}
		queued := state == "running" && response.StatusCode == http.StatusOK && result.Success != nil && !*result.Success && result.Error == "resources_unavailable" && strings.Contains(strings.ToLower(result.Message), "state change queued")
		if queued {
			reason = "request queued — " + details
		}
		return &StateChangeError{message: fmt.Sprintf("%s: %s", prefix, reason), queued: queued}
	}
	return nil
}
