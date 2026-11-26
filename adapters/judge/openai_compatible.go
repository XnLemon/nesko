package judge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxModelResponseBytes = 2 << 20

// OpenAICompatibleConfig configures an endpoint that implements Chat Completions.
type OpenAICompatibleConfig struct {
	Endpoint   string
	Model      string
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// OpenAICompatibleClient sends non-streaming chat completion requests.
type OpenAICompatibleClient struct {
	endpoint string
	model    string
	apiKey   string
	client   *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatCompletionResponse struct {
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
	Choices []struct {
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// NewOpenAICompatibleClient validates endpoint configuration and applies safe timeouts.
func NewOpenAICompatibleClient(config OpenAICompatibleConfig) (*OpenAICompatibleClient, error) {
	endpoint := strings.TrimSpace(config.Endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("judge endpoint must be an absolute HTTP(S) URL without user info or fragment")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("judge model is required")
	}
	client := config.HTTPClient
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAICompatibleClient{
		endpoint: endpoint,
		model:    strings.TrimSpace(config.Model),
		apiKey:   strings.TrimSpace(config.APIKey),
		client:   client,
	}, nil
}

// Complete sends a system and user message and returns the first choice's text content.
func (c *OpenAICompatibleClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body, err := json.Marshal(chatCompletionRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("encode judge model request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create judge model request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send judge model request: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read judge model response: %w", err)
	}
	if len(responseBody) > maxModelResponseBytes {
		return "", fmt.Errorf("judge model response exceeds %d bytes", maxModelResponseBytes)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("judge model returned HTTP %d: %s", response.StatusCode, compactErrorBody(responseBody))
	}

	var completion chatCompletionResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return "", fmt.Errorf("decode judge model response: %w", err)
	}
	if completion.Error != nil {
		return "", fmt.Errorf("judge model error: %s", completion.Error.Message)
	}
	if len(completion.Choices) == 0 {
		return "", errors.New("judge model response has no choices")
	}
	content, err := decodeMessageContent(completion.Choices[0].Message.Content)
	if err != nil {
		return "", fmt.Errorf("decode judge model content: %w", err)
	}
	return content, nil
}

func decodeMessageContent(raw json.RawMessage) (string, error) {
	var content string
	if err := json.Unmarshal(raw, &content); err == nil {
		return content, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return "", errors.New("message content is neither text nor text parts")
	}
	var result strings.Builder
	for _, part := range parts {
		if part.Type == "text" || part.Type == "output_text" {
			result.WriteString(part.Text)
		}
	}
	if result.Len() == 0 {
		return "", errors.New("message content has no text parts")
	}
	return result.String(), nil
}

func compactErrorBody(body []byte) string {
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		return strings.TrimSpace(parsed.Error.Message)
	}
	const maxErrorBytes = 512
	value := strings.TrimSpace(string(body))
	if len(value) > maxErrorBytes {
		value = value[:maxErrorBytes] + "…"
	}
	return value
}
