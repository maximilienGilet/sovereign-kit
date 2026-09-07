package setup

import (
	"encoding/json"
	"strings"
)

// DownloadProgress reports bytes received, not model readiness. Total zero
// means the response did not supply a trustworthy size.
type DownloadProgress struct {
	Current int64
	Total   int64
}

func parseDownloadMarker(line string) (*DownloadProgress, bool) {
	payload, ok := strings.CutPrefix(strings.TrimSpace(line), "SOVKIT_DOWNLOAD ")
	if !ok {
		return nil, false
	}
	var value struct {
		Current *int64 `json:"current"`
		Total   *int64 `json:"total"`
	}
	if json.Unmarshal([]byte(payload), &value) != nil || value.Current == nil || value.Total == nil || *value.Current < 0 || *value.Total < 0 {
		return nil, true
	}
	total := *value.Total
	if total < *value.Current {
		total = 0
	}
	return &DownloadProgress{Current: *value.Current, Total: total}, true
}

// ParseDownloadProgress returns the last measured download in a log snapshot.
// Invalid newer markers clear older progress, as do subsequent loading stages.
func ParseDownloadProgress(text string) *DownloadProgress {
	var latest *DownloadProgress
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Verifying GGUF") || strings.HasPrefix(line, "Launching llama-server:") {
			latest = nil
			continue
		}
		value, ok := parseDownloadMarker(line)
		if !ok {
			continue
		}
		latest = nil
		if value == nil {
			continue
		}
		latest = value
	}
	return latest
}

// ParseDownloadHighWater returns the furthest trustworthy measured transfer
// in a snapshot, even when a later line has moved on to verification. It is
// used only to retain visual completion; ParseDownloadProgress remains the
// authority for whether a transfer is currently active.
func ParseDownloadHighWater(text string) *DownloadProgress {
	var high *DownloadProgress
	for _, line := range strings.Split(text, "\n") {
		value, marker := parseDownloadMarker(line)
		if !marker || value == nil || value.Total <= 0 || value.Current > value.Total {
			continue
		}
		if high == nil || float64(value.Current)/float64(value.Total) > float64(high.Current)/float64(high.Total) {
			high = value
		}
	}
	return high
}
