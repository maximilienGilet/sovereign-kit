package vast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type SearchRequest struct {
	Limit         int
	MinimumVRAMGB int
}

type Offer struct {
	ID          int     `json:"id"`
	GPUName     string  `json:"gpu_name"`
	GPUVRAMGB   float64 `json:"gpu_ram"`
	HourlyUSD   float64 `json:"dph_total"`
	Location    string  `json:"geolocation"`
	Reliability float64 `json:"reliability"`
}

type searchResponse struct {
	Offers json.RawMessage `json:"offers"`
}

// SearchOffers returns on-demand, verified, rentable offers with enough VRAM
// for the selected built-in profile. It never creates an instance.
func (client *Client) SearchOffers(ctx context.Context, request SearchRequest) ([]Offer, error) {
	if request.Limit < 1 || request.Limit > 100 {
		return nil, fmt.Errorf("offer limit must be between 1 and 100")
	}
	if request.MinimumVRAMGB < 1 {
		return nil, fmt.Errorf("minimum VRAM must be positive")
	}
	if client.token == "" {
		return nil, fmt.Errorf("Vast API token is required")
	}
	payload := map[string]any{
		"limit": request.Limit,
		"type":  "on-demand",
		"verified": map[string]bool{"eq": true},
		"rentable": map[string]bool{"eq": true},
		"rented": map[string]bool{"eq": false},
		"gpu_ram": map[string]int{"gte": request.MinimumVRAMGB},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/api/v0/bundles", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(httpRequest)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Vast search offers returned HTTP %d", response.StatusCode)
	}
	var result searchResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Vast offer response: %w", err)
	}
	var offers []Offer
	if err := json.Unmarshal(result.Offers, &offers); err == nil {
		return offers, nil
	}
	var one Offer
	if err := json.Unmarshal(result.Offers, &one); err != nil {
		return nil, fmt.Errorf("decode Vast offers: %w", err)
	}
	return []Offer{one}, nil
}
