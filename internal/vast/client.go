// Package vast contains the narrow Vast.ai instance-creation adapter.
package vast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/maximilienGilet/sovereign-kit/internal/sshkey"
	"net/http"
	"strings"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Instance struct {
	DaemonLogs     string `json:"-"`
	Image          string `json:"image_uuid"`
	StatusMessage  string `json:"status_msg"`
	ID             int    `json:"id"`
	Status         string `json:"actual_status"`
	IntendedStatus string `json:"intended_status"`
	NextState      string `json:"next_state"`
	SSHHost        string `json:"ssh_host"`
	SSHPort        int    `json:"ssh_port"`
}

func (instance Instance) StartRequested() bool {
	return strings.EqualFold(instance.IntendedStatus, "running") || strings.EqualFold(instance.NextState, "running")
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
	if err := decodeSingleJSON(response.Body, &result); err != nil {
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

type sshKeyRecord struct {
	Key       string `json:"key"`
	SSHKey    string `json:"ssh_key"`
	PublicKey string `json:"public_key"`
}

type sshKeysEnvelope struct {
	SSHKeys []sshKeyRecord `json:"ssh_keys"`
}

func (client *Client) HasSSHKey(ctx context.Context, publicKey string) (bool, error) {
	if strings.TrimSpace(client.token) == "" {
		return false, fmt.Errorf("Vast API token is required")
	}
	normalized, err := sshkey.Normalize(publicKey)
	if err != nil {
		return false, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/api/v0/ssh/", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode == http.StatusForbidden {
		return false, fmt.Errorf("Vast list SSH keys returned HTTP 403; VAST_API_KEY must grant user_read at https://cloud.vast.ai/manage-keys/")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return false, fmt.Errorf("Vast list SSH keys returned HTTP %d", response.StatusCode)
	}
	raw := json.RawMessage{}
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return false, fmt.Errorf("decode Vast SSH keys response: %w", err)
	}
	var records []sshKeyRecord
	if bytes.HasPrefix(bytes.TrimSpace(raw), []byte("[")) {
		if err := json.Unmarshal(raw, &records); err != nil {
			return false, fmt.Errorf("decode Vast SSH keys response: %w", err)
		}
	} else {
		var envelope sshKeysEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return false, fmt.Errorf("decode Vast SSH keys response: %w", err)
		}
		records = envelope.SSHKeys
	}
	for _, record := range records {
		candidate := record.Key
		if strings.TrimSpace(candidate) == "" {
			candidate = record.SSHKey
		}
		if strings.TrimSpace(candidate) == "" {
			candidate = record.PublicKey
		}
		value, err := sshkey.Normalize(candidate)
		if err == nil && value == normalized {
			return true, nil
		}
	}
	return false, nil
}

func (client *Client) AddSSHKey(ctx context.Context, publicKey string) error {
	if strings.TrimSpace(client.token) == "" {
		return fmt.Errorf("Vast API token is required")
	}
	if _, err := sshkey.Normalize(publicKey); err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		SSHKey string `json:"ssh_key"`
	}{SSHKey: strings.TrimSpace(publicKey)})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+"/api/v0/ssh/", bytes.NewReader(body))
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
	if response.StatusCode == http.StatusForbidden {
		return fmt.Errorf("Vast create SSH key returned HTTP 403; VAST_API_KEY must grant user_write at https://cloud.vast.ai/manage-keys/")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Vast create SSH key returned HTTP %d", response.StatusCode)
	}
	return nil
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
