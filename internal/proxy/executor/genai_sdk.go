package executor

import (
	"context"
	"fmt"
	"net/http"

	"google.golang.org/genai"
)

// GenAIClientFactory creates an official Google GenAI SDK client for Gemini or Vertex AI.
func NewGenAIClient(ctx context.Context, backend genai.Backend, apiKey, project, location string, httpClient *http.Client) (*genai.Client, error) {
	cfg := &genai.ClientConfig{
		Backend:    backend,
		APIKey:     apiKey,
		Project:    project,
		Location:   location,
		HTTPClient: httpClient,
	}
	client, err := genai.NewClient(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("genai sdk client creation: %w", err)
	}
	return client, nil
}
