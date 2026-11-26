package judge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const openAIResponsesEndpoint = "https://api.openai.com/v1/responses"

// AccessTokenSource supplies a bearer token for an OpenAI Responses API request.
type AccessTokenSource interface {
	AccessToken(ctx context.Context) (string, error)
}

// OpenAIResponsesConfig configures Judge requests using the OpenAI Responses API.
type OpenAIResponsesConfig struct {
	Model             string
	AccessTokenSource AccessTokenSource
	HTTPClient        *http.Client
	Timeout           time.Duration
}

// OpenAIResponsesClient sends OAuth-authenticated, streamed Responses API requests.
type OpenAIResponsesClient struct {
	model       string
	tokenSource AccessTokenSource
	client      *http.Client
}

type responsesRequest struct {
	Model        string                  `json:"model"`
	Instructions string                  `json:"instructions"`
	Input        []responsesInputMessage `json:"input"`
	Store        bool                    `json:"store"`
	Stream       bool                    `json:"stream"`
}

type responsesInputMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responsesStreamEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Response struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"response"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewOpenAIResponsesClient validates model and token-source configuration.
func NewOpenAIResponsesClient(config OpenAIResponsesConfig) (*OpenAIResponsesClient, error) {
	if strings.TrimSpace(config.Model) == "" {
		return nil, errors.New("judge model is required")
	}
	if config.AccessTokenSource == nil {
		return nil, errors.New("judge access token source is required")
	}
	client := config.HTTPClient
	if client == nil {
		timeout := config.Timeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAIResponsesClient{
		model:       strings.TrimSpace(config.Model),
		tokenSource: config.AccessTokenSource,
		client:      client,
	}, nil
}

// Complete sends a streamed Responses API request and returns completed output text.
func (c *OpenAIResponsesClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	token, err := c.tokenSource.AccessToken(ctx)
	if err != nil {
		return "", fmt.Errorf("get Judge access token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return "", errors.New("Judge access token is empty")
	}

	body, err := json.Marshal(responsesRequest{
		Model:        c.model,
		Instructions: systemPrompt,
		Input:        []responsesInputMessage{{Role: "user", Content: userPrompt}},
		Store:        false,
		Stream:       true,
	})
	if err != nil {
		return "", fmt.Errorf("encode Judge Responses request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, openAIResponsesEndpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create Judge Responses request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")

	response, err := c.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("send Judge Responses request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxModelResponseBytes+1))
		if err != nil {
			return "", fmt.Errorf("read Judge Responses error: %w", err)
		}
		if len(responseBody) > maxModelResponseBytes {
			return "", fmt.Errorf("Judge Responses error exceeds %d bytes", maxModelResponseBytes)
		}
		return "", fmt.Errorf("Judge Responses API returned HTTP %d: %s", response.StatusCode, compactErrorBody(responseBody))
	}
	return readResponsesStream(response.Body)
}

func readResponsesStream(reader io.Reader) (string, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, min(64<<10, maxModelResponseBytes)), maxModelResponseBytes)
	var output strings.Builder
	var totalBytes int
	completed := false
	for scanner.Scan() {
		totalBytes += len(scanner.Bytes()) + 1
		if totalBytes > maxModelResponseBytes {
			return "", fmt.Errorf("Judge Responses stream exceeds %d bytes", maxModelResponseBytes)
		}
		if err := processResponsesStreamLine(scanner.Bytes(), &output, &completed); err != nil {
			return "", err
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read Judge Responses stream: %w", err)
	}
	if !completed {
		return "", errors.New("Judge Responses stream ended before response.completed")
	}
	if output.Len() == 0 {
		return "", errors.New("Judge Responses completed without output text")
	}
	return output.String(), nil
}

func processResponsesStreamLine(line []byte, output *strings.Builder, completed *bool) error {
	if !bytes.HasPrefix(line, []byte("data:")) {
		return nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(string(line), "data:"))
	if data == "" || data == "[DONE]" {
		return nil
	}
	var event responsesStreamEvent
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return fmt.Errorf("decode Judge Responses stream event: %w", err)
	}
	return handleResponsesStreamEvent(event, output, completed)
}

func handleResponsesStreamEvent(event responsesStreamEvent, output *strings.Builder, completed *bool) error {
	switch event.Type {
	case "response.output_text.delta":
		if output.Len()+len(event.Delta) > maxModelResponseBytes {
			return fmt.Errorf("Judge Responses output exceeds %d bytes", maxModelResponseBytes)
		}
		output.WriteString(event.Delta)
	case "response.failed":
		return fmt.Errorf("Judge Responses request failed: %s", responseStreamError(event))
	case "response.incomplete":
		return fmt.Errorf("Judge Responses request incomplete: %s", responseStreamError(event))
	case "response.completed":
		*completed = true
	}
	return nil
}

func responseStreamError(event responsesStreamEvent) string {
	if event.Response.Error != nil {
		return formatResponseError(event.Response.Error.Code, event.Response.Error.Message)
	}
	if event.Error != nil {
		return formatResponseError(event.Error.Code, event.Error.Message)
	}
	return "unknown error"
}

func formatResponseError(code, message string) string {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	if code == "" {
		if message == "" {
			return "unknown error"
		}
		return message
	}
	if message == "" {
		return code
	}
	return code + ": " + message
}

var _ CompletionClient = (*OpenAIResponsesClient)(nil)
