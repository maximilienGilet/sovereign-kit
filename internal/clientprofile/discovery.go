// Package clientprofile discovers the private endpoint and manages opt-in isolated profiles.
package clientprofile

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

type Metadata struct {
	ID            string `toml:"id"`
	ContextWindow int    `toml:"context_window"`
	MaxTokens     int    `toml:"max_tokens"`
}
type Endpoint struct {
	BaseURL string
	Metadata
	Problem       string
	LimitsProblem string
}

func (e Endpoint) Installable() bool {
	return e.Problem == "" && validID(e.ID) && e.ContextWindow > 0 && e.MaxTokens > 0 && e.MaxTokens <= e.ContextWindow
}
func validID(id string) bool {
	return strings.TrimSpace(id) == id && id != "" && !strings.ContainsFunc(id, unicode.IsControl)
}

// Discover accepts limits only from matching saved deployment metadata. OpenAI's
// models response does not standardize usable context/output limits.
func Discover(ctx context.Context, base string, saved Metadata) Endpoint {
	e := Endpoint{BaseURL: strings.TrimRight(base, "/")}
	u, err := url.Parse(e.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		e.Problem = "Invalid configured endpoint"
		return e
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, e.BaseURL+"/models", nil)
	if err != nil {
		e.Problem = err.Error()
		return e
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		e.Problem = "Model discovery unavailable: " + err.Error()
		return e
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		e.Problem = fmt.Sprintf("Model discovery returned HTTP %d", response.StatusCode)
		return e
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(raw) > 1<<20 {
		e.Problem = "Model response is unreadable or too large"
		return e
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		e.Problem = "Invalid model response"
		return e
	}
	ids := map[string]bool{}
	for _, m := range result.Data {
		if !validID(m.ID) {
			e.Problem = "Invalid model identifier"
			return e
		}
		ids[m.ID] = true
	}
	if len(ids) != 1 {
		e.Problem = "Expected one unique model; endpoint returned none or multiple models"
		return e
	}
	for id := range ids {
		e.ID = id
	}
	if saved.ID == e.ID && saved.ContextWindow > 0 && saved.MaxTokens > 0 && saved.MaxTokens <= saved.ContextWindow {
		e.Metadata = saved
	} else {
		e.LimitsProblem = "Verified context and output limits are missing for this model; profile installation is unavailable."
	}
	return e
}
