// Package huggingface resolves a model reference into a pinned, classified artifact.
package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/maximilienGilet/sovereign-kit/internal/catalog"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Model struct {
	Repository     string
	Revision       string
	Classification catalog.Result
}

type SearchResult struct {
	Repository string `json:"id"`
	Downloads  int    `json:"downloads"`
	Likes      int    `json:"likes"`
}

type modelResponse struct {
	ID          string   `json:"id"`
	SHA         string   `json:"sha"`
	PipelineTag string   `json:"pipeline_tag"`
	Tags        []string `json:"tags"`
	Siblings    []struct {
		Filename string `json:"rfilename"`
	} `json:"siblings"`
}

type modelConfig struct {
	AutoMap map[string]json.RawMessage `json:"auto_map"`
}

func (client *Client) requiresRemoteCode(ctx context.Context, repository, revision string) (bool, error) {
	parts := strings.Split(repository, "/")
	endpoint := fmt.Sprintf("%s/%s/%s/resolve/%s/config.json", client.baseURL, url.PathEscape(parts[0]), url.PathEscape(parts[1]), url.PathEscape(revision))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("Hugging Face pinned model config returned HTTP %d", response.StatusCode)
	}
	var config modelConfig
	if err := json.NewDecoder(response.Body).Decode(&config); err != nil {
		return false, fmt.Errorf("decode Hugging Face pinned model config: %w", err)
	}
	return len(config.AutoMap) > 0, nil
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: http.DefaultClient}
}

func (client *Client) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("Hugging Face search query is required")
	}
	if limit < 1 {
		return nil, fmt.Errorf("Hugging Face search limit must be positive")
	}
	endpoint, err := url.Parse(client.baseURL + "/api/models")
	if err != nil {
		return nil, err
	}
	values := endpoint.Query()
	values.Set("search", query)
	values.Set("sort", "downloads")
	values.Set("direction", "-1")
	values.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Hugging Face model search returned HTTP %d", response.StatusCode)
	}
	var results []SearchResult
	if err := json.NewDecoder(response.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("decode Hugging Face model search: %w", err)
	}
	return results, nil
}

// Inspect resolves the current Hub revision once. The caller must show it for
// confirmation and persist the SHA, never a mutable branch name.
func (client *Client) Inspect(ctx context.Context, repository string) (Model, error) {
	if !validRepository(repository) {
		return Model{}, fmt.Errorf("repository must be owner/name")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/api/models/"+repository, nil)
	if err != nil {
		return Model{}, err
	}
	response, err := client.http.Do(request)
	if err != nil {
		return Model{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Model{}, fmt.Errorf("Hugging Face model inspection returned HTTP %d", response.StatusCode)
	}
	var payload modelResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Model{}, fmt.Errorf("decode Hugging Face model metadata: %w", err)
	}
	if payload.ID == "" || payload.SHA == "" {
		return Model{}, fmt.Errorf("Hugging Face response has no immutable model revision")
	}
	files := make([]string, 0, len(payload.Siblings))
	for _, sibling := range payload.Siblings {
		files = append(files, sibling.Filename)
	}
	requiresRemoteCode, err := client.requiresRemoteCode(ctx, payload.ID, payload.SHA)
	if err != nil {
		return Model{}, err
	}
	return Model{Repository: payload.ID, Revision: payload.SHA, Classification: catalog.Classify(catalog.Artifact{
		Repository: payload.ID, PipelineTag: payload.PipelineTag, Tags: payload.Tags, Files: files, RequiresRemoteCode: requiresRemoteCode,
	})}, nil
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(repository, " \t\n?#")
}
