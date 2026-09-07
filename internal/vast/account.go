package vast

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
)

const maxAccountResponseBytes = 256 * 1024

type accountResponse struct {
	Balance *float64 `json:"balance"`
	Credit  *float64 `json:"credit"`
}

// Balance returns the current authenticated Vast credit balance.
func (client *Client) Balance(ctx context.Context) (float64, error) {
	if strings.TrimSpace(client.token) == "" {
		return 0, fmt.Errorf("Vast API token is required")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/api/v0/users/current/", nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("Vast show user returned HTTP %d", response.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxAccountResponseBytes+1))
	if err != nil {
		return 0, fmt.Errorf("read Vast account response: %w", err)
	}
	if len(payload) > maxAccountResponseBytes {
		return 0, fmt.Errorf("Vast account response exceeds %d bytes", maxAccountResponseBytes)
	}
	var result accountResponse
	if err := decodeSingleJSON(bytes.NewReader(payload), &result); err != nil {
		return 0, fmt.Errorf("decode Vast account response: %w", err)
	}
	// Vast's console displays spendable credit for ordinary non-negative
	// accounts. Balance is a distinct billing field and is only its fallback.
	amount := result.Credit
	if amount == nil {
		amount = result.Balance
	}
	if result.Credit != nil && result.Balance != nil && (*result.Credit < 0 || *result.Balance < 0) {
		amount = result.Balance
	}
	if amount == nil || math.IsNaN(*amount) || math.IsInf(*amount, 0) || *amount < 0 {
		return 0, fmt.Errorf("Vast account response has no valid balance")
	}
	return *amount, nil
}
