// Package huggingface resolves a model reference into a pinned, classified artifact.
package huggingface

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

type modelResponse struct {
	ID          string   `json:"id"`
	SHA         string   `json:"sha"`
	PipelineTag string   `json:"pipeline_tag"`
	Tags        []string `json:"tags"`
	Siblings    []struct {
		Filename string `json:"rfilename"`
	} `json:"siblings"`
}

func NewClient(baseURL string) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), http: http.DefaultClient}
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
	return Model{Repository: payload.ID, Revision: payload.SHA, Classification: catalog.Classify(catalog.Artifact{
		Repository: payload.ID, PipelineTag: payload.PipelineTag, Tags: payload.Tags, Files: files,
	})}, nil
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(repository, " \t\n?#")
}
