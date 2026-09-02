// Package vast contains the narrow Vast.ai instance-creation adapter.
package vast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Instance struct {
	ID      int    `json:"id"`
	Status  string `json:"actual_status"`
	SSHHost string `json:"ssh_host"`
	SSHPort int    `json:"ssh_port"`
}

type instanceResponse struct {
	Instance Instance `json:"instances"`
}

func (client *Client) GetInstance(ctx context.Context, instanceID int) (Instance, error) {
	if strings.TrimSpace(client.token) == "" {
		return Instance{}, fmt.Errorf("Vast API token is required")
	}
	if instanceID <= 0 {
		return Instance{}, fmt.Errorf("Vast instance ID must be positive")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v0/instances/%d", client.baseURL, instanceID), nil)
	if err != nil {
		return Instance{}, err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return Instance{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return Instance{}, fmt.Errorf("Vast show instance returned HTTP %d", response.StatusCode)
	}
	var result instanceResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return Instance{}, fmt.Errorf("decode Vast instance response: %w", err)
	}
	if result.Instance.ID != instanceID {
		return Instance{}, fmt.Errorf("Vast response did not return instance %d", instanceID)
	}
	return result.Instance, nil
}

type CreateRequest struct {
	Image  string
	DiskGB int
	Label  string
}

type createPayload struct {
	Image   string `json:"image"`
	DiskGB  int    `json:"disk"`
	Runtype string `json:"runtype"`
	Label   string `json:"label,omitempty"`
}

type createResponse struct {
	Success     bool `json:"success"`
	NewContract int  `json:"new_contract"`
}

func NewClient(baseURL, token string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: http.DefaultClient}
}

func isDigestImage(image string) bool {
	digest := strings.TrimSpace(image)
	separator := strings.LastIndex(digest, "@sha256:")
	if separator < 1 || len(digest)-separator-8 != 64 {
		return false
	}
	for _, character := range digest[separator+8:] {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

// CreateInstance accepts one selected Vast offer. It does not search, destroy, or
// change any existing instance.
func (client *Client) CreateInstance(ctx context.Context, offerID int, request CreateRequest) (int, error) {
	if strings.TrimSpace(client.token) == "" {
		return 0, fmt.Errorf("Vast API token is required")
	}
	if offerID <= 0 {
		return 0, fmt.Errorf("Vast offer ID must be positive")
	}
	if strings.TrimSpace(request.Image) == "" {
		return 0, fmt.Errorf("Vast image is required")
	}
	if !isDigestImage(request.Image) {
		return 0, fmt.Errorf("Vast image must be pinned by sha256 digest")
	}
	if request.DiskGB <= 0 {
		return 0, fmt.Errorf("Vast disk size must be positive")
	}
	body, err := json.Marshal(createPayload{
		Image: request.Image, DiskGB: request.DiskGB, Runtype: "ssh_direct", Label: request.Label,
	})
	if err != nil {
		return 0, err
	}
	url := fmt.Sprintf("%s/api/v0/asks/%d", client.baseURL, offerID)
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+client.token)
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := client.http.Do(httpRequest)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("Vast create instance returned HTTP %d", response.StatusCode)
	}
	var result createResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decode Vast create-instance response: %w", err)
	}
	if !result.Success || result.NewContract <= 0 {
		return 0, fmt.Errorf("Vast did not return a new instance ID")
	}
	return result.NewContract, nil
}
