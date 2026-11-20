package modelgateway

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

var (
	ErrInvalidResponse = errors.New("invalid model gateway response")
	ErrUnknownOutcome  = errors.New("model gateway outcome is unknown")
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type Request struct {
	Model           string    `json:"model"`
	Messages        []Message `json:"messages"`
	MaxOutputTokens int64     `json:"max_tokens,omitempty"`
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

type Response struct {
	Text  string
	Usage Usage
}

type wireResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

func New(baseURL, apiKey string, httpClient *http.Client) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("model gateway base URL must be an absolute HTTP(S) URL without credentials or query")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("model gateway API key is required")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 90 * time.Second}
	}
	return &Client{baseURL: baseURL, apiKey: apiKey, http: httpClient}, nil
}

func (c *Client) Call(ctx context.Context, modelID string, request Request) (Response, error) {
	if strings.TrimSpace(modelID) == "" || request.Model != "" && request.Model != modelID {
		return Response{}, fmt.Errorf("%w: model is not server controlled", ErrInvalidResponse)
	}
	request.Model = modelID
	requestBody, err := json.Marshal(request)
	if err != nil {
		return Response{}, fmt.Errorf("encode model request: %w", err)
	}
	endpoint := c.endpoint()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return Response{}, fmt.Errorf("create model request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	response, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) {
			return Response{}, ErrUnknownOutcome
		}
		return Response{}, fmt.Errorf("call model gateway: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		if response.StatusCode >= http.StatusInternalServerError {
			return Response{}, ErrUnknownOutcome
		}
		return Response{}, fmt.Errorf("model gateway returned HTTP %d", response.StatusCode)
	}
	var payload wireResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return Response{}, fmt.Errorf("decode model gateway response: %w", err)
	}
	if len(payload.Choices) == 0 || payload.Usage == nil || payload.Usage.PromptTokens < 0 || payload.Usage.CompletionTokens < 0 || payload.Usage.TotalTokens < 0 || payload.Usage.TotalTokens != payload.Usage.PromptTokens+payload.Usage.CompletionTokens {
		return Response{}, ErrInvalidResponse
	}
	return Response{Text: payload.Choices[0].Message.Content, Usage: *payload.Usage}, nil
}

func (c *Client) endpoint() string {
	base := strings.TrimRight(c.baseURL, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/chat/completions"
	}
	return base + "/v1/chat/completions"
}
