package vast

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DestroyInstance removes one exact Vast instance after the caller has chosen
// that instance. Vast confirms a successful destruction with HTTP 200 and a
// success flag; every other response is an error.
func (client *Client) DestroyInstance(ctx context.Context, instanceID int) error {
	if strings.TrimSpace(client.token) == "" {
		return fmt.Errorf("Vast API token is required")
	}
	if instanceID <= 0 {
		return fmt.Errorf("Vast instance ID must be positive")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, fmt.Sprintf("%s/api/v0/instances/%d", client.baseURL, instanceID), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Vast destroy instance returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Success bool `json:"success"`
	}
	if err := decodeSingleJSON(response.Body, &result); err != nil {
		return fmt.Errorf("decode Vast destroy-instance response: %w", err)
	}
	if !result.Success {
		return fmt.Errorf("Vast did not confirm instance destruction")
	}
	return nil
}

// InstanceExists establishes existence only from the documented targeted
// endpoint. It reports absence only for an explicit JSON null instances field.
func (client *Client) InstanceExists(ctx context.Context, instanceID int) (bool, error) {
	if strings.TrimSpace(client.token) == "" {
		return false, fmt.Errorf("Vast API token is required")
	}
	if instanceID <= 0 {
		return false, fmt.Errorf("Vast instance ID must be positive")
	}
	endpoint := fmt.Sprintf("%s/api/v0/instances/%d/?%s", client.baseURL, instanceID, url.Values{"owner": {"me"}}.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+client.token)
	response, err := client.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return false, fmt.Errorf("Vast instance lookup returned HTTP %d", response.StatusCode)
	}
	var result struct {
		Instances json.RawMessage `json:"instances"`
	}
	if err := decodeSingleJSON(response.Body, &result); err != nil {
		return false, fmt.Errorf("decode Vast instance lookup response: %w", err)
	}
	if len(result.Instances) == 0 {
		return false, fmt.Errorf("Vast instance lookup did not include instances")
	}
	if bytes.Equal(bytes.TrimSpace(result.Instances), []byte("null")) {
		return false, nil
	}
	var instance struct {
		ID *int `json:"id"`
	}
	if err := json.Unmarshal(result.Instances, &instance); err != nil {
		return false, fmt.Errorf("decode Vast instance lookup result: %w", err)
	}
	if instance.ID == nil || *instance.ID != instanceID {
		return false, fmt.Errorf("Vast response did not return instance %d", instanceID)
	}
	return true, nil
}

func decodeSingleJSON(reader io.Reader, value any) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}
